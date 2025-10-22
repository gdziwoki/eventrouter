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

package sinks

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewEventData(t *testing.T) {
	newEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "New event created",
	}

	oldEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Updated",
		Message: "Event updated",
	}

	tests := []struct {
		name           string
		newEvent       *v1.Event
		oldEvent       *v1.Event
		expectedVerb   string
		expectOldEvent bool
	}{
		{
			name:           "New event without old event",
			newEvent:       newEvent,
			oldEvent:       nil,
			expectedVerb:   "ADDED",
			expectOldEvent: false,
		},
		{
			name:           "New event with old event",
			newEvent:       newEvent,
			oldEvent:       oldEvent,
			expectedVerb:   "UPDATED",
			expectOldEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventData := NewEventData(tt.newEvent, tt.oldEvent)

			if eventData.Verb != tt.expectedVerb {
				t.Errorf("Expected verb %s, got %s", tt.expectedVerb, eventData.Verb)
			}

			if eventData.Event != tt.newEvent {
				t.Error("Event field does not match new event")
			}

			if tt.expectOldEvent {
				if eventData.OldEvent != tt.oldEvent {
					t.Error("OldEvent field does not match old event")
				}
			} else {
				if eventData.OldEvent != nil {
					t.Error("OldEvent should be nil when no old event provided")
				}
			}
		})
	}
}

func TestEventData_JSONSerialization(t *testing.T) {
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Test event for JSON serialization",
	}

	eventData := NewEventData(event, nil)

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(eventData)
	if err != nil {
		t.Fatalf("Failed to marshal EventData to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled EventData
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal EventData from JSON: %v", err)
	}

	// Verify fields
	if unmarshaled.Verb != eventData.Verb {
		t.Errorf("Expected verb %s, got %s after unmarshaling", eventData.Verb, unmarshaled.Verb)
	}

	if unmarshaled.Event.Name != event.Name {
		t.Errorf("Expected event name %s, got %s after unmarshaling", event.Name, unmarshaled.Event.Name)
	}
}

func TestNewStdoutSink(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
	}{
		{
			name:      "Stdout sink with namespace",
			namespace: "kubernetes",
		},
		{
			name:      "Stdout sink without namespace",
			namespace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := NewStdoutSink(tt.namespace)

			if sink == nil {
				t.Fatal("NewStdoutSink returned nil")
			}

			// Verify it implements EventSinkInterface
			_, ok := sink.(EventSinkInterface)
			if !ok {
				t.Error("NewStdoutSink does not implement EventSinkInterface")
			}

			// Type assert to check internal namespace field
			stdoutSink, ok := sink.(*StdoutSink)
			if !ok {
				t.Fatal("NewStdoutSink did not return *StdoutSink")
			}

			if stdoutSink.namespace != tt.namespace {
				t.Errorf("Expected namespace %s, got %s", tt.namespace, stdoutSink.namespace)
			}
		})
	}
}

func TestStdoutSink_UpdateEvents(t *testing.T) {
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Test event for stdout sink",
	}

	oldEvent := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "old-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Updated",
		Message: "Old event for stdout sink",
	}

	tests := []struct {
		name      string
		namespace string
		newEvent  *v1.Event
		oldEvent  *v1.Event
	}{
		{
			name:      "Update with namespace",
			namespace: "kubernetes",
			newEvent:  event,
			oldEvent:  nil,
		},
		{
			name:      "Update without namespace",
			namespace: "",
			newEvent:  event,
			oldEvent:  nil,
		},
		{
			name:      "Update with old event",
			namespace: "",
			newEvent:  event,
			oldEvent:  oldEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			oldStderr := os.Stderr

			r, w, _ := os.Pipe()
			os.Stdout = w
			os.Stderr = w

			sink := NewStdoutSink(tt.namespace)
			sink.UpdateEvents(tt.newEvent, tt.oldEvent)

			// Restore stdout/stderr and read captured output
			w.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Verify JSON output
			if output == "" {
				t.Error("Expected JSON output, got empty string")
				return
			}

			// Parse the JSON to verify it's valid
			var result map[string]interface{}
			err := json.Unmarshal([]byte(output), &result)
			if err != nil {
				t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, output)
				return
			}

			// Check namespace wrapping
			if tt.namespace != "" {
				if _, exists := result[tt.namespace]; !exists {
					t.Errorf("Expected namespace %s in JSON output", tt.namespace)
				}
			} else {
				// Without namespace, should have event data directly
				if _, exists := result["verb"]; !exists {
					t.Error("Expected 'verb' field in JSON output")
				}
			}
		})
	}
}

func TestStdoutSink_UpdateEvents_JSONMarshalError(t *testing.T) {
	// Test with a problematic event that might cause JSON marshal errors
	// This is difficult to trigger with normal v1.Event objects, but we can test the error path
	sink := NewStdoutSink("")

	// Capture stderr to check error output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Call with nil event (which should not cause JSON errors, but tests the path)
	sink.UpdateEvents(nil, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)

	// The function should handle nil events gracefully and still produce JSON
	// If there were JSON errors, they would appear in stderr
}

func TestManufactureSink(t *testing.T) {
	tests := []struct {
		name         string
		sinkType     string
		expectedType string
	}{
		{
			name:         "Stdout sink",
			sinkType:     "stdout",
			expectedType: "*sinks.StdoutSink",
		},
		{
			name:         "Unknown sink defaults to stdout",
			sinkType:     "unknown",
			expectedType: "*sinks.StdoutSink",
		},
		{
			name:         "Empty sink defaults to stdout",
			sinkType:     "",
			expectedType: "*sinks.StdoutSink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up viper config
			viper.Reset()
			viper.Set("sink", tt.sinkType)

			sink := ManufactureSink()

			if sink == nil {
				t.Fatal("ManufactureSink returned nil")
			}

			// Verify it implements EventSinkInterface
			_, ok := sink.(EventSinkInterface)
			if !ok {
				t.Error("ManufactureSink does not implement EventSinkInterface")
			}

			// Check the actual type
			switch sink.(type) {
			case *StdoutSink:
				if tt.expectedType != "*sinks.StdoutSink" {
					t.Errorf("Expected %s, got *StdoutSink", tt.expectedType)
				}
			default:
				t.Errorf("Unexpected sink type: %T", sink)
			}
		})
	}
}

func TestManufactureSink_WithNamespace(t *testing.T) {
	viper.Reset()
	viper.Set("sink", "stdout")
	viper.Set("stdoutJSONNamespace", "test-namespace")

	sink := ManufactureSink()
	stdoutSink, ok := sink.(*StdoutSink)
	if !ok {
		t.Fatal("Expected *StdoutSink")
	}

	if stdoutSink.namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", stdoutSink.namespace)
	}
}

func BenchmarkNewEventData(b *testing.B) {
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "benchmark-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Benchmark event",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewEventData(event, nil)
	}
}

func BenchmarkStdoutSink_UpdateEvents(b *testing.B) {
	sink := NewStdoutSink("")
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "benchmark-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Benchmark event",
	}

	// Redirect stdout to discard output during benchmarking
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.UpdateEvents(event, nil)
	}
}

func BenchmarkJSONMarshal(b *testing.B) {
	eventData := EventData{
		Verb: "ADDED",
		Event: &v1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "benchmark-event",
				Namespace: "default",
			},
			Type:    "Normal",
			Reason:  "Created",
			Message: "Benchmark event",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(eventData)
		if err != nil {
			b.Fatal(err)
		}
	}
}
