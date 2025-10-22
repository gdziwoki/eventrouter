/*
Copyright 2017 Heptio Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

// MockSink implements EventSinkInterface for testing
type MockSink struct {
	events    []*v1.Event
	oldEvents []*v1.Event
	callCount int
}

func (m *MockSink) UpdateEvents(eNew *v1.Event, eOld *v1.Event) {
	m.events = append(m.events, eNew)
	if eOld != nil {
		m.oldEvents = append(m.oldEvents, eOld)
	}
	m.callCount++
}

func (m *MockSink) GetEvents() []*v1.Event {
	return m.events
}

func (m *MockSink) GetOldEvents() []*v1.Event {
	return m.oldEvents
}

func (m *MockSink) GetCallCount() int {
	return m.callCount
}

func (m *MockSink) Reset() {
	m.events = nil
	m.oldEvents = nil
	m.callCount = 0
}

func TestEventRouter_shouldProcessEvent(t *testing.T) {
	tests := []struct {
		name                    string
		lastSeenResourceVersion string
		resourceVersion         string
		expected                bool
	}{
		{
			name:                    "Empty resource version should not be processed",
			lastSeenResourceVersion: "100",
			resourceVersion:         "",
			expected:                false,
		},
		{
			name:                    "First event with empty last seen should be processed",
			lastSeenResourceVersion: "",
			resourceVersion:         "100",
			expected:                true,
		},
		{
			name:                    "Newer resource version should be processed",
			lastSeenResourceVersion: "100",
			resourceVersion:         "200",
			expected:                true,
		},
		{
			name:                    "Older resource version should not be processed",
			lastSeenResourceVersion: "200",
			resourceVersion:         "100",
			expected:                false,
		},
		{
			name:                    "Same resource version should not be processed",
			lastSeenResourceVersion: "100",
			resourceVersion:         "100",
			expected:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			er := &EventRouter{
				lastSeenResourceVersion: tt.lastSeenResourceVersion,
			}

			result := er.shouldProcessEvent(tt.resourceVersion)
			if result != tt.expected {
				t.Errorf("shouldProcessEvent() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestEventRouter_addEvent(t *testing.T) {
	mockSink := &MockSink{}
	var capturedResourceVersion string

	er := &EventRouter{
		eSink:                   mockSink,
		lastSeenResourceVersion: "100",
		lastResourceVersionPosition: func(rv string) {
			capturedResourceVersion = rv
		},
	}

	tests := []struct {
		name          string
		event         interface{}
		expectError   bool
		expectProcess bool
	}{
		{
			name: "Valid event with newer resource version",
			event: &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-event",
					ResourceVersion: "200",
				},
				Type:    "Normal",
				Reason:  "Created",
				Message: "Test event",
			},
			expectError:   false,
			expectProcess: true,
		},
		{
			name: "Valid event with older resource version",
			event: &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "old-event",
					ResourceVersion: "50",
				},
				Type:    "Normal",
				Reason:  "Created",
				Message: "Old test event",
			},
			expectError:   false,
			expectProcess: false,
		},
		{
			name:          "Invalid event type",
			event:         "not-an-event",
			expectError:   true,
			expectProcess: false,
		},
		{
			name:          "Nil event",
			event:         (*v1.Event)(nil),
			expectError:   true,
			expectProcess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSink.Reset()
			capturedResourceVersion = ""

			er.addEvent(tt.event)

			if tt.expectProcess {
				if mockSink.GetCallCount() != 1 {
					t.Errorf("Expected sink to be called once, got %d calls", mockSink.GetCallCount())
				}

				events := mockSink.GetEvents()
				if len(events) != 1 {
					t.Errorf("Expected 1 event in sink, got %d", len(events))
				} else {
					expectedEvent := tt.event.(*v1.Event)
					if events[0].Name != expectedEvent.Name {
						t.Errorf("Expected event name %s, got %s", expectedEvent.Name, events[0].Name)
					}
				}

				if capturedResourceVersion != tt.event.(*v1.Event).ResourceVersion {
					t.Errorf("Expected resource version %s to be captured, got %s",
						tt.event.(*v1.Event).ResourceVersion, capturedResourceVersion)
				}
			} else {
				if mockSink.GetCallCount() != 0 {
					t.Errorf("Expected sink not to be called, got %d calls", mockSink.GetCallCount())
				}
			}
		})
	}
}

func TestEventRouter_updateEvent(t *testing.T) {
	mockSink := &MockSink{}
	var capturedResourceVersion string

	er := &EventRouter{
		eSink:                   mockSink,
		lastSeenResourceVersion: "100",
		lastResourceVersionPosition: func(rv string) {
			capturedResourceVersion = rv
		},
	}

	oldEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-event",
			ResourceVersion: "150",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Old message",
	}

	newEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-event",
			ResourceVersion: "200",
		},
		Type:    "Normal",
		Reason:  "Updated",
		Message: "New message",
	}

	tests := []struct {
		name        string
		oldEvent    interface{}
		newEvent    interface{}
		expectError bool
		expectCall  bool
	}{
		{
			name:        "Valid event update",
			oldEvent:    oldEvent,
			newEvent:    newEvent,
			expectError: false,
			expectCall:  true,
		},
		{
			name:        "Invalid old event type",
			oldEvent:    "not-an-event",
			newEvent:    newEvent,
			expectError: true,
			expectCall:  false,
		},
		{
			name:        "Invalid new event type",
			oldEvent:    oldEvent,
			newEvent:    "not-an-event",
			expectError: true,
			expectCall:  false,
		},
		{
			name:        "Nil new event",
			oldEvent:    oldEvent,
			newEvent:    (*v1.Event)(nil),
			expectError: true,
			expectCall:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSink.Reset()
			capturedResourceVersion = ""

			er.updateEvent(tt.oldEvent, tt.newEvent)

			if tt.expectCall {
				if mockSink.GetCallCount() != 1 {
					t.Errorf("Expected sink to be called once, got %d calls", mockSink.GetCallCount())
				}

				events := mockSink.GetEvents()
				oldEvents := mockSink.GetOldEvents()

				if len(events) != 1 {
					t.Errorf("Expected 1 new event in sink, got %d", len(events))
				}

				if len(oldEvents) != 1 {
					t.Errorf("Expected 1 old event in sink, got %d", len(oldEvents))
				}

				if capturedResourceVersion != newEvent.ResourceVersion {
					t.Errorf("Expected resource version %s to be captured, got %s",
						newEvent.ResourceVersion, capturedResourceVersion)
				}
			} else {
				if mockSink.GetCallCount() != 0 {
					t.Errorf("Expected sink not to be called, got %d calls", mockSink.GetCallCount())
				}
			}
		})
	}
}

func TestEventRouter_deleteEvent(t *testing.T) {
	er := &EventRouter{}

	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		Reason: "Deleted",
	}

	tests := []struct {
		name  string
		event interface{}
	}{
		{
			name:  "Valid event deletion",
			event: event,
		},
		{
			name:  "Invalid event type",
			event: "not-an-event",
		},
		{
			name:  "Nil event",
			event: (*v1.Event)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deleteEvent just logs, so we test that it doesn't panic
			er.deleteEvent(tt.event)
		})
	}
}

func TestPrometheusEvent(t *testing.T) {
	// Enable prometheus for testing
	viper.Set("enable-prometheus", true)

	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		Type:   "Normal",
		Reason: "Created",
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: "default",
		},
		Source: v1.EventSource{
			Host: "test-host",
		},
	}

	tests := []struct {
		name  string
		event *v1.Event
	}{
		{
			name:  "Normal event",
			event: event,
		},
		{
			name: "Warning event",
			event: &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "warning-event",
					Namespace: "default",
				},
				Type:   "Warning",
				Reason: "Failed",
				InvolvedObject: v1.ObjectReference{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
				Source: v1.EventSource{
					Host: "test-host",
				},
			},
		},
		{
			name: "Info event",
			event: &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "info-event",
					Namespace: "default",
				},
				Type:   "Info",
				Reason: "Info",
				InvolvedObject: v1.ObjectReference{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
				Source: v1.EventSource{
					Host: "test-host",
				},
			},
		},
		{
			name: "Unknown event type",
			event: &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unknown-event",
					Namespace: "default",
				},
				Type:   "Unknown",
				Reason: "Unknown",
				InvolvedObject: v1.ObjectReference{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
				Source: v1.EventSource{
					Host: "test-host",
				},
			},
		},
		{
			name:  "Nil event",
			event: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// prometheusEvent should not panic with any input
			prometheusEvent(tt.event)
		})
	}

	// Test with prometheus disabled
	t.Run("Prometheus disabled", func(t *testing.T) {
		viper.Set("enable-prometheus", false)
		prometheusEvent(event)
		// Should not panic
	})
}

func TestNewEventRouter(t *testing.T) {
	// Create a fake clientset for testing
	clientset := fake.NewSimpleClientset()

	// Create shared informer factory
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)
	eventsInformer := informerFactory.Core().V1().Events()

	lastSeenResourceVersion := "100"
	var capturedResourceVersion string
	resourceVersionFunc := func(rv string) {
		capturedResourceVersion = rv
	}

	// Enable prometheus for testing
	viper.Set("enable-prometheus", true)

	// Create EventRouter
	er := NewEventRouter(clientset, eventsInformer, lastSeenResourceVersion, resourceVersionFunc)

	// Verify EventRouter is properly initialized
	if er == nil {
		t.Fatal("NewEventRouter returned nil")
	}

	if er.kubeClient != clientset {
		t.Error("EventRouter kubeClient not set correctly")
	}

	if er.lastSeenResourceVersion != lastSeenResourceVersion {
		t.Errorf("Expected lastSeenResourceVersion %s, got %s",
			lastSeenResourceVersion, er.lastSeenResourceVersion)
	}

	if er.eSink == nil {
		t.Error("EventRouter sink not initialized")
	}

	// Test that resource version function works
	testVersion := "200"
	resourceVersionFunc(testVersion)
	if capturedResourceVersion != testVersion {
		t.Errorf("Expected captured resource version %s, got %s",
			testVersion, capturedResourceVersion)
	}
}

func TestEventRouter_Run(t *testing.T) {
	// Create a fake clientset
	clientset := fake.NewSimpleClientset()

	// Create shared informer factory
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)
	eventsInformer := informerFactory.Core().V1().Events()

	// Create EventRouter
	er := NewEventRouter(clientset, eventsInformer, "", func(string) {})

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	stopCh := ctx.Done()

	// Start the informer factory
	informerFactory.Start(stopCh)

	// Run EventRouter in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		er.Run(stopCh)
	}()

	// Wait a bit to let it start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the EventRouter
	cancel()

	// Wait for EventRouter to finish
	select {
	case <-done:
		// EventRouter finished successfully
	case <-time.After(5 * time.Second):
		t.Fatal("EventRouter did not stop within timeout")
	}
}

func BenchmarkEventRouter_addEvent(b *testing.B) {
	mockSink := &MockSink{}
	er := &EventRouter{
		eSink:                       mockSink,
		lastSeenResourceVersion:     "0",
		lastResourceVersionPosition: func(string) {},
	}

	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "benchmark-event",
			ResourceVersion: "1000000",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Benchmark event",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		er.addEvent(event)
	}
}

func BenchmarkPrometheusEvent(b *testing.B) {
	// Enable prometheus for benchmarking
	viper.Set("enable-prometheus", true)

	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "benchmark-event",
			Namespace: "default",
		},
		Type:   "Normal",
		Reason: "Created",
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Name:      "benchmark-pod",
			Namespace: "default",
		},
		Source: v1.EventSource{
			Host: "benchmark-host",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prometheusEvent(event)
	}
}
