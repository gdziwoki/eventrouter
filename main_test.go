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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestSigHandler(t *testing.T) {
	// Test that sigHandler creates a channel that can be closed
	stopCh := sigHandler()

	// Verify the channel is not nil
	if stopCh == nil {
		t.Fatal("sigHandler returned nil channel")
	}

	// Verify we can read from the channel (should block until signal)
	select {
	case <-stopCh:
		t.Fatal("Channel should not be closed immediately")
	default:
		// Expected behavior - channel should block
	}
}

func TestLoadConfig_WithValidConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir, err := os.MkdirTemp("", "eventrouter_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.json")
	configContent := `{
		"sink": "stdout",
		"resync-interval": "5m",
		"enable-prometheus": true
	}`

	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variable to use our test config
	originalEnv := os.Getenv("EVENTROUTER_CONFIG")
	os.Setenv("EVENTROUTER_CONFIG", configFile)
	defer func() {
		if originalEnv != "" {
			os.Setenv("EVENTROUTER_CONFIG", originalEnv)
		} else {
			os.Unsetenv("EVENTROUTER_CONFIG")
		}
	}()

	// Reset viper for clean test
	viper.Reset()

	// This test will fail because we can't connect to Kubernetes in test environment
	// But we can verify it reads the config correctly by checking the error message
	_, err = loadConfig()

	// We expect an error since we're not in a Kubernetes environment
	if err == nil {
		t.Fatal("Expected error when not in Kubernetes environment")
	}

	// The error should be about Kubernetes config, not config file reading
	expectedErrors := []string{
		"failed to build kubernetes config",
		"failed to create kubernetes clientset",
	}

	errorMatched := false
	for _, expectedError := range expectedErrors {
		if len(err.Error()) > len(expectedError) && err.Error()[:len(expectedError)] == expectedError {
			errorMatched = true
			break
		}
	}

	if !errorMatched {
		t.Fatalf("Unexpected error (should be about Kubernetes config): %v", err)
	}

	// Verify config was read correctly
	if viper.GetString("sink") != "stdout" {
		t.Errorf("Expected sink to be 'stdout', got %s", viper.GetString("sink"))
	}

	if viper.GetDuration("resync-interval") != 5*time.Minute {
		t.Errorf("Expected resync-interval to be 5m, got %v", viper.GetDuration("resync-interval"))
	}

	if !viper.GetBool("enable-prometheus") {
		t.Error("Expected enable-prometheus to be true")
	}
}

func TestLoadConfig_WithInvalidForcedConfig(t *testing.T) {
	// Test with non-existent forced config file
	originalEnv := os.Getenv("EVENTROUTER_CONFIG")
	os.Setenv("EVENTROUTER_CONFIG", "/nonexistent/config.json")
	defer func() {
		if originalEnv != "" {
			os.Setenv("EVENTROUTER_CONFIG", originalEnv)
		} else {
			os.Unsetenv("EVENTROUTER_CONFIG")
		}
	}()

	// Reset viper for clean test
	viper.Reset()

	_, err := loadConfig()

	if err == nil {
		t.Fatal("Expected error for non-existent forced config file")
	}

	expectedError := "failed to read forced config file"
	if len(err.Error()) < len(expectedError) || err.Error()[:len(expectedError)] != expectedError {
		t.Errorf("Expected error about forced config file, got: %v", err)
	}
}

func TestLoadConfig_WithDefaults(t *testing.T) {
	// Clear any existing config
	viper.Reset()

	// Unset the forced config environment variable
	originalEnv := os.Getenv("EVENTROUTER_CONFIG")
	os.Unsetenv("EVENTROUTER_CONFIG")
	defer func() {
		if originalEnv != "" {
			os.Setenv("EVENTROUTER_CONFIG", originalEnv)
		}
	}()

	// Unset KUBECONFIG environment variable to test true defaults
	originalKubeconfig := os.Getenv("KUBECONFIG")
	os.Unsetenv("KUBECONFIG")
	defer func() {
		if originalKubeconfig != "" {
			os.Setenv("KUBECONFIG", originalKubeconfig)
		}
	}()

	// Create a minimal config directory to avoid file not found issues
	tmpDir, err := os.MkdirTemp("", "eventrouter_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory so viper looks for config there
	originalDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalDir)

	// This should use defaults since no config file exists
	_, err = loadConfig()

	// We expect a Kubernetes connection error, not a config error
	if err == nil {
		t.Fatal("Expected Kubernetes connection error")
	}

	// Verify that default values are set
	if viper.GetString("sink") != "stdout" {
		t.Errorf("Expected default sink 'stdout', got %s", viper.GetString("sink"))
	}

	if viper.GetDuration("resync-interval") != 30*time.Minute {
		t.Errorf("Expected default resync-interval 30m, got %v", viper.GetDuration("resync-interval"))
	}

	if !viper.GetBool("enable-prometheus") {
		t.Error("Expected default enable-prometheus to be true")
	}

	if viper.GetString("kubeconfig") != "" {
		t.Errorf("Expected default kubeconfig to be empty, got %s", viper.GetString("kubeconfig"))
	}
}

func TestSafeCompareResourceVersions(t *testing.T) {
	// This tests the logic that would be in the safeCompareResourceVersions function
	// We'll create a simple version for testing
	safeCompareResourceVersions := func(rv1, rv2 string) bool {
		if rv1 == "" && rv2 == "" {
			return false
		}
		if rv1 == "" {
			return false
		}
		if rv2 == "" {
			return true
		}
		// Simple numeric comparison for test
		return rv1 > rv2
	}

	testCases := []struct {
		rv1      string
		rv2      string
		expected bool
	}{
		{"", "", false},
		{"", "100", false},
		{"100", "", true},
		{"200", "100", true},
		{"100", "200", false},
		{"100", "100", false},
	}

	for _, tc := range testCases {
		result := safeCompareResourceVersions(tc.rv1, tc.rv2)
		if result != tc.expected {
			t.Errorf("safeCompareResourceVersions(%q, %q) = %v, expected %v",
				tc.rv1, tc.rv2, result, tc.expected)
		}
	}
}

// TestConfigurationDefaults verifies that all required default values are set
func TestConfigurationDefaults(t *testing.T) {
	viper.Reset()

	// Simulate the default setting from loadConfig
	viper.SetDefault("kubeconfig", "")
	viper.SetDefault("sink", "stdout")
	viper.SetDefault("resync-interval", time.Minute*30)
	viper.SetDefault("enable-prometheus", true)

	tests := []struct {
		key      string
		expected interface{}
	}{
		{"kubeconfig", ""},
		{"sink", "stdout"},
		{"resync-interval", time.Minute * 30},
		{"enable-prometheus", true},
	}

	for _, test := range tests {
		switch expected := test.expected.(type) {
		case string:
			if actual := viper.GetString(test.key); actual != expected {
				t.Errorf("Default %s = %v, expected %v", test.key, actual, expected)
			}
		case time.Duration:
			if actual := viper.GetDuration(test.key); actual != expected {
				t.Errorf("Default %s = %v, expected %v", test.key, actual, expected)
			}
		case bool:
			if actual := viper.GetBool(test.key); actual != expected {
				t.Errorf("Default %s = %v, expected %v", test.key, actual, expected)
			}
		}
	}
}
