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
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/heptiolabs/eventrouter/sinks"
	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BenchmarkSink implements EventSinkInterface for benchmarking
type BenchmarkSink struct {
	processedCount int64
}

func (b *BenchmarkSink) UpdateEvents(eNew *v1.Event, eOld *v1.Event) {
	b.processedCount++
}

func (b *BenchmarkSink) GetProcessedCount() int64 {
	return b.processedCount
}

func (b *BenchmarkSink) Reset() {
	b.processedCount = 0
}

// createBenchmarkEvent creates a realistic test event for benchmarking
func createBenchmarkEvent(name, resourceVersion string) *v1.Event {
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "default",
			ResourceVersion: resourceVersion,
			Labels: map[string]string{
				"app":     "test-app",
				"version": "v1.0.0",
			},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
		},
		Type:    "Normal",
		Reason:  "Created",
		Message: "Pod test-pod was created successfully with image nginx:1.20",
		Count:   1,
		InvolvedObject: v1.ObjectReference{
			Kind:       "Pod",
			Name:       "test-pod",
			Namespace:  "default",
			APIVersion: "v1",
			UID:        "12345678-1234-1234-1234-123456789012",
		},
		Source: v1.EventSource{
			Component: "kubelet",
			Host:      "worker-node-1",
		},
		FirstTimestamp: metav1.Now(),
		LastTimestamp:  metav1.Now(),
	}
}

func BenchmarkEventRouter_ProcessingSingleEvent(b *testing.B) {
	benchSink := &BenchmarkSink{}
	er := &EventRouter{
		eSink:                       benchSink,
		lastSeenResourceVersion:     "0",
		lastResourceVersionPosition: func(string) {},
	}

	event := createBenchmarkEvent("benchmark-event", "1000000")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		er.addEvent(event)
	}

	b.StopTimer()
	b.Logf("Processed %d events", benchSink.GetProcessedCount())
}

func BenchmarkEventRouter_ProcessingMultipleEvents(b *testing.B) {
	benchSink := &BenchmarkSink{}
	er := &EventRouter{
		eSink:                       benchSink,
		lastSeenResourceVersion:     "0",
		lastResourceVersionPosition: func(string) {},
	}

	// Create multiple events for more realistic benchmarking
	events := make([]*v1.Event, 100)
	for i := 0; i < 100; i++ {
		events[i] = createBenchmarkEvent("benchmark-event", "1000000")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, event := range events {
			er.addEvent(event)
		}
	}

	b.StopTimer()
	b.Logf("Processed %d events total", benchSink.GetProcessedCount())
}

func BenchmarkEventRouter_ShouldProcessEvent(b *testing.B) {
	er := &EventRouter{
		lastSeenResourceVersion: "500000",
	}

	testCases := []string{
		"500001", // Should process
		"499999", // Should not process
		"500000", // Should not process (equal)
		"600000", // Should process
		"100000", // Should not process
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rv := testCases[i%len(testCases)]
		er.shouldProcessEvent(rv)
	}
}

func BenchmarkStdoutSink_JSONSerialization(b *testing.B) {
	// Redirect stdout to prevent console spam during benchmarking
	oldStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	sink := sinks.NewStdoutSink("")
	event := createBenchmarkEvent("json-benchmark-event", "1000000")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sink.UpdateEvents(event, nil)
	}
}

func BenchmarkStdoutSink_JSONSerializationWithNamespace(b *testing.B) {
	// Redirect stdout to prevent console spam during benchmarking
	oldStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	defer func() {
		os.Stdout = oldStdout
		devNull.Close()
	}()

	sink := sinks.NewStdoutSink("kubernetes")
	event := createBenchmarkEvent("json-namespace-benchmark-event", "1000000")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sink.UpdateEvents(event, nil)
	}
}

func BenchmarkEventData_Creation(b *testing.B) {
	event := createBenchmarkEvent("eventdata-benchmark", "1000000")
	oldEvent := createBenchmarkEvent("old-eventdata-benchmark", "999999")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			sinks.NewEventData(event, nil)
		} else {
			sinks.NewEventData(event, oldEvent)
		}
	}
}

func BenchmarkJSON_MarshalEvent(b *testing.B) {
	event := createBenchmarkEvent("json-marshal-benchmark", "1000000")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(event)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSON_MarshalEventData(b *testing.B) {
	event := createBenchmarkEvent("json-marshal-eventdata-benchmark", "1000000")
	eventData := sinks.NewEventData(event, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(eventData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrometheusEvent_AllTypes(b *testing.B) {
	// Enable prometheus for benchmarking
	viper.Set("enable-prometheus", true)

	events := []*v1.Event{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "normal-event", Namespace: "default"},
			Type:       "Normal", Reason: "Created",
			InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "test-pod", Namespace: "default"},
			Source:         v1.EventSource{Host: "test-host"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "warning-event", Namespace: "default"},
			Type:       "Warning", Reason: "Failed",
			InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "test-pod", Namespace: "default"},
			Source:         v1.EventSource{Host: "test-host"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "info-event", Namespace: "default"},
			Type:       "Info", Reason: "Info",
			InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "test-pod", Namespace: "default"},
			Source:         v1.EventSource{Host: "test-host"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "unknown-event", Namespace: "default"},
			Type:       "Unknown", Reason: "Unknown",
			InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "test-pod", Namespace: "default"},
			Source:         v1.EventSource{Host: "test-host"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		event := events[i%len(events)]
		prometheusEvent(event)
	}
}

func BenchmarkResourceVersionComparison(b *testing.B) {
	// Test the performance of resource version comparison using string conversion
	testPairs := [][2]string{
		{"100", "200"},
		{"999999", "1000000"},
		{"1", "999999"},
		{"500000", "500001"},
		{"123456", "123455"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pair := testPairs[i%len(testPairs)]
		rv1, rv2 := pair[0], pair[1]

		// Simulate the comparison logic used in shouldProcessEvent
		_ = rv1 > rv2 // String comparison as currently done
	}
}

// BenchmarkMemoryUsage tests memory allocation patterns
func BenchmarkMemoryUsage_EventProcessing(b *testing.B) {
	benchSink := &BenchmarkSink{}
	er := &EventRouter{
		eSink:                       benchSink,
		lastSeenResourceVersion:     "0",
		lastResourceVersionPosition: func(string) {},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create a new event each time to measure allocation overhead
		event := createBenchmarkEvent("memory-benchmark", "1000000")
		er.addEvent(event)
	}

	b.StopTimer()

	// Report memory statistics
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Process additional events to see memory growth
	for i := 0; i < 1000; i++ {
		event := createBenchmarkEvent("memory-test", "1000000")
		er.addEvent(event)
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	b.Logf("Memory usage - TotalAllocs: %d, Sys: %d KB",
		m2.TotalAlloc-m1.TotalAlloc, (m2.Sys-m1.Sys)/1024)
}
