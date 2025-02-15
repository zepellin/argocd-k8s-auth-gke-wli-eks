package logger

import (
	"errors"
	"testing"
)

func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "basic configuration",
			config: Config{
				Level:     1,
				Verbosity: 1,
			},
			wantErr: false,
		},
		{
			name: "with file output",
			config: Config{
				Level:     2,
				ToFile:    "/tmp/test.log",
				Verbosity: 2,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	// Initialize with test configuration
	config := Config{
		Level:     1,
		Verbosity: 1,
	}
	if err := Initialize(config); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Test each logging function
	// Note: These tests only verify that the functions don't panic
	// since klog writes to stderr by default
	t.Run("Error", func(t *testing.T) {
		Error("test error message")
	})

	t.Run("Errorf", func(t *testing.T) {
		Errorf(errors.New("test error"), "error occurred: %s", "test")
	})

	t.Run("Warning", func(t *testing.T) {
		Warning("test warning message")
	})

	t.Run("Info", func(t *testing.T) {
		Info("test info message")
	})

	t.Run("Infof", func(t *testing.T) {
		Infof("test info message", "key1", "value1", "key2", "value2")
	})

	t.Run("Debug", func(t *testing.T) {
		Debug("test debug message")
	})

	t.Run("V", func(t *testing.T) {
		if !V(1) {
			t.Error("Expected V(1) to return true with verbosity level 1")
		}
		if V(2) {
			t.Error("Expected V(2) to return false with verbosity level 1")
		}
	})

	// Test Flush
	t.Run("Flush", func(t *testing.T) {
		Flush()
	})
}
