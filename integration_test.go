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
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

// IntegrationMockSink captures events for integration testing
type IntegrationMockSink struct {
	mu           sync.RWMutex
	events       []v1.Event
	oldEvents    []v1.Event
	callCount    int
	processedRVs []string
}

func (m *IntegrationMockSink) UpdateEvents(eNew *v1.Event, eOld *v1.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if eNew != nil {
		m.events = append(m.events, *eNew)
		m.processedRVs = append(m.processedRVs, eNew.ResourceVersion)
	}
	if eOld != nil {
		m.oldEvents = append(m.oldEvents, *eOld)
	}
	m.callCount++
}

func (m *IntegrationMockSink) GetEvents() []v1.Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]v1.Event{}, m.events...)
}

func (m *IntegrationMockSink) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

func (m *IntegrationMockSink) GetProcessedResourceVersions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.processedRVs...)
}

func (m *IntegrationMockSink) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
	m.oldEvents = nil
	m.processedRVs = nil
	m.callCount = 0
}

func TestIntegration_EventProcessingFlow(t *testing.T) {
	// Create fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create informer factory with short resync period for testing
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second)
	eventsInformer := informerFactory.Core().V1().Events()

	// Create integration mock sink
	mockSink := &IntegrationMockSink{}

	// Track resource version updates
	var resourceVersions []string
	var rvMutex sync.Mutex
	resourceVersionFunc := func(rv string) {
		rvMutex.Lock()
		resourceVersions = append(resourceVersions, rv)
		rvMutex.Unlock()
	}

	// Create EventRouter with our mock sink
	er := NewEventRouter(clientset, eventsInformer, "", resourceVersionFunc)

	// Replace the manufactured sink with our test sink
	er.eSink = mockSink

	// Create context for controlling the test
	ctx, cancel := context.WithCancel(context.Background())
	stopCh := ctx.Done()

	// Start the informer factory
	informerFactory.Start(stopCh)

	// Start EventRouter in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		er.Run(stopCh)
	}()

	// Wait for informers to sync
	syncResults := informerFactory.WaitForCacheSync(stopCh)
	for informer, synced := range syncResults {
		if !synced {
			t.Fatalf("Failed to sync informer cache for %v", informer)
		}
	}

	// Create test events in the fake clientset
	events := []*v1.Event{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "event1",
				Namespace:       "default",
				ResourceVersion: "1000",
			},
			Type:    "Normal",
			Reason:  "Created",
			Message: "First test event",
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "test-pod-1",
				Namespace: "default",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "event2",
				Namespace:       "default",
				ResourceVersion: "1001",
			},
			Type:    "Warning",
			Reason:  "Failed",
			Message: "Second test event",
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "test-pod-2",
				Namespace: "default",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "event3",
				Namespace:       "kube-system",
				ResourceVersion: "1002",
			},
			Type:    "Normal",
			Reason:  "Scheduled",
			Message: "Third test event",
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Name:      "system-pod",
				Namespace: "kube-system",
			},
		},
	}

	// Add events to the fake clientset
	for _, event := range events {
		_, err := clientset.CoreV1().Events(event.Namespace).Create(
			context.Background(), event, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}
	}

	// Wait for events to be processed
	maxWait := time.Second * 5
	timeout := time.After(maxWait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var processedEvents []v1.Event
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for events to be processed. Got %d events, expected %d",
				len(processedEvents), len(events))
		case <-ticker.C:
			processedEvents = mockSink.GetEvents()
			if len(processedEvents) >= len(events) {
				goto eventsProcessed
			}
		}
	}

eventsProcessed:
	// Stop the EventRouter
	cancel()
	wg.Wait()

	// Verify all events were processed
	if len(processedEvents) != len(events) {
		t.Errorf("Expected %d events, got %d", len(events), len(processedEvents))
	}

	// Verify events are in the correct order and have correct content
	for i, processedEvent := range processedEvents {
		expectedEvent := events[i]
		if processedEvent.Name != expectedEvent.Name {
			t.Errorf("Event %d: expected name %s, got %s", i, expectedEvent.Name, processedEvent.Name)
		}
		if processedEvent.Reason != expectedEvent.Reason {
			t.Errorf("Event %d: expected reason %s, got %s", i, expectedEvent.Reason, processedEvent.Reason)
		}
		if processedEvent.Type != expectedEvent.Type {
			t.Errorf("Event %d: expected type %s, got %s", i, expectedEvent.Type, processedEvent.Type)
		}
	}

	// Verify resource versions were tracked
	rvMutex.Lock()
	processedRVs := resourceVersions
	rvMutex.Unlock()

	if len(processedRVs) != len(events) {
		t.Errorf("Expected %d resource version updates, got %d", len(events), len(processedRVs))
	}

	for i, rv := range processedRVs {
		expectedRV := events[i].ResourceVersion
		if rv != expectedRV {
			t.Errorf("Resource version %d: expected %s, got %s", i, expectedRV, rv)
		}
	}
}

func TestIntegration_EventUpdateFlow(t *testing.T) {
	// Create fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second)
	eventsInformer := informerFactory.Core().V1().Events()

	// Create integration mock sink
	mockSink := &IntegrationMockSink{}

	// Create EventRouter with our mock sink
	er := NewEventRouter(clientset, eventsInformer, "", func(string) {})
	er.eSink = mockSink

	// Create context for controlling the test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopCh := ctx.Done()

	// Start the informer factory and EventRouter
	informerFactory.Start(stopCh)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		er.Run(stopCh)
	}()

	// Wait for informers to sync
	syncResults := informerFactory.WaitForCacheSync(stopCh)
	for informer, synced := range syncResults {
		if !synced {
			t.Fatalf("Failed to sync informer cache for %v", informer)
		}
	}

	// Create an initial event
	initialEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "update-test-event",
			Namespace:       "default",
			ResourceVersion: "2000",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Initial event message",
		Count:   1,
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Name:      "update-test-pod",
			Namespace: "default",
		},
	}

	// Create the initial event
	createdEvent, err := clientset.CoreV1().Events("default").Create(
		context.Background(), initialEvent, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create initial event: %v", err)
	}

	// Wait for initial event to be processed
	time.Sleep(100 * time.Millisecond)

	// Update the event
	createdEvent.Message = "Updated event message"
	createdEvent.Count = 2
	createdEvent.ResourceVersion = "2001" // Simulate resource version update

	_, err = clientset.CoreV1().Events("default").Update(
		context.Background(), createdEvent, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update event: %v", err)
	}

	// Wait for update to be processed
	time.Sleep(100 * time.Millisecond)

	// Stop EventRouter
	cancel()
	wg.Wait()

	// Verify both create and update were processed
	processedEvents := mockSink.GetEvents()
	if len(processedEvents) < 1 {
		t.Fatal("Expected at least 1 processed event")
	}

	// The last processed event should have the updated message
	lastEvent := processedEvents[len(processedEvents)-1]
	if lastEvent.Message != "Updated event message" {
		t.Errorf("Expected updated message, got: %s", lastEvent.Message)
	}
}

func TestIntegration_ResourceVersionFiltering(t *testing.T) {
	// Create fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second)
	eventsInformer := informerFactory.Core().V1().Events()

	// Create integration mock sink
	mockSink := &IntegrationMockSink{}

	// Start with a higher resource version to test filtering
	startingResourceVersion := "5000"

	// Create EventRouter
	er := NewEventRouter(clientset, eventsInformer, startingResourceVersion, func(string) {})
	er.eSink = mockSink

	// Create context for controlling the test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopCh := ctx.Done()

	// Start the informer factory and EventRouter
	informerFactory.Start(stopCh)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		er.Run(stopCh)
	}()

	// Wait for informers to sync
	syncResults := informerFactory.WaitForCacheSync(stopCh)
	for informer, synced := range syncResults {
		if !synced {
			t.Fatalf("Failed to sync informer cache for %v", informer)
		}
	}

	// Create events with different resource versions
	testEvents := []*v1.Event{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "old-event",
				Namespace:       "default",
				ResourceVersion: "4999", // Older than starting RV, should be filtered
			},
			Type:    "Normal",
			Reason:  "Old",
			Message: "This should be filtered out",
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "new-event",
				Namespace:       "default",
				ResourceVersion: "5001", // Newer than starting RV, should be processed
			},
			Type:    "Normal",
			Reason:  "New",
			Message: "This should be processed",
		},
	}

	// Add events to the fake clientset
	for _, event := range testEvents {
		_, err := clientset.CoreV1().Events(event.Namespace).Create(
			context.Background(), event, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create event: %v", err)
		}
	}

	// Wait for events to be processed
	time.Sleep(200 * time.Millisecond)

	// Stop EventRouter
	cancel()
	wg.Wait()

	// Verify only the newer event was processed
	processedEvents := mockSink.GetEvents()

	// Should only process the event with RV > 5000
	expectedCount := 1
	if len(processedEvents) != expectedCount {
		t.Errorf("Expected %d processed events, got %d", expectedCount, len(processedEvents))
	}

	if len(processedEvents) > 0 {
		if processedEvents[0].Reason != "New" {
			t.Errorf("Expected 'New' event to be processed, got event with reason: %s", processedEvents[0].Reason)
		}
	}
}

func BenchmarkIntegration_EventProcessing(b *testing.B) {
	// Create fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Hour) // Long resync for benchmark

	// Create EventRouter with stdout sink for realistic benchmarking
	eventsInformer := informerFactory.Core().V1().Events()
	er := NewEventRouter(clientset, eventsInformer, "", func(string) {})

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start informers (but don't wait for sync in benchmark)
	informerFactory.Start(ctx.Done())

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		event := &v1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "benchmark-event",
				Namespace:       "default",
				ResourceVersion: "1000000", // High RV to ensure processing
			},
			Type:    "Normal",
			Reason:  "Benchmark",
			Message: "Benchmark event processing",
		}

		// Simulate event processing directly (bypass informer for pure processing benchmark)
		er.addEvent(event)
	}
}
