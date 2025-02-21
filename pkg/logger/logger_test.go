package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
)

type testBuffer struct {
	buf bytes.Buffer
}

func (b *testBuffer) Write(p []byte) (n int, err error) {
	return b.buf.Write(p)
}

func (b *testBuffer) String() string {
	return b.buf.String()
}

type logEntry struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Error string `json:"error,omitempty"`
	Time  string `json:"time"`
}

func TestInitialize(t *testing.T) {
	tempDir := t.TempDir()

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
				ToFile:    tempDir + "/test.log",
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

			if tt.config.ToFile != "" {
				if _, err := os.Stat(tt.config.ToFile); os.IsNotExist(err) {
					t.Errorf("Expected log file %s to exist", tt.config.ToFile)
				}
			}
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	buf := &testBuffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger = slog.New(handler)

	tests := []struct {
		name      string
		logFunc   func()
		wantLevel string
		wantMsg   string
		wantErr   string
	}{
		{
			name:      "Error",
			logFunc:   func() { Error("test error message") },
			wantLevel: "ERROR",
			wantMsg:   "test error message",
		},
		{
			name:      "Errorf",
			logFunc:   func() { Errorf(errors.New("test error"), "error occurred: %s", "test") },
			wantLevel: "ERROR",
			wantMsg:   "error occurred: test",
			wantErr:   "test error",
		},
		{
			name:      "Warning",
			logFunc:   func() { Warning("test warning message") },
			wantLevel: "WARN",
			wantMsg:   "test warning message",
		},
		{
			name:      "Info",
			logFunc:   func() { Info("test info message") },
			wantLevel: "INFO",
			wantMsg:   "test info message",
		},
		{
			name:      "Debug",
			logFunc:   func() { Debug("test debug message") },
			wantLevel: "DEBUG",
			wantMsg:   "test debug message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.buf.Reset()
			tt.logFunc()

			var entry logEntry
			if err := json.Unmarshal(buf.buf.Bytes(), &entry); err != nil {
				t.Fatalf("Failed to parse log entry: %v", err)
			}

			if entry.Level != tt.wantLevel {
				t.Errorf("got level %q, want %q", entry.Level, tt.wantLevel)
			}

			if entry.Msg != tt.wantMsg {
				t.Errorf("got message %q, want %q", entry.Msg, tt.wantMsg)
			}

			if tt.wantErr != "" && !strings.Contains(entry.Error, tt.wantErr) {
				t.Errorf("got error %q, want to contain %q", entry.Error, tt.wantErr)
			}
		})
	}

	// Test Infof separately as it has a different structure
	t.Run("Infof", func(t *testing.T) {
		buf.buf.Reset()
		Infof("test message", "key1", "value1")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.buf.Bytes(), &entry); err != nil {
			t.Fatalf("Failed to parse log entry: %v", err)
		}

		if entry["level"] != "INFO" {
			t.Errorf("got level %v, want INFO", entry["level"])
		}

		if entry["msg"] != "test message" {
			t.Errorf("got message %v, want 'test message'", entry["msg"])
		}

		if entry["key1"] != "value1" {
			t.Errorf("got key1=%v, want 'value1'", entry["key1"])
		}
	})

	// Test V function
	t.Run("V level tests", func(t *testing.T) {
		tests := []struct {
			level    int
			config   Config
			expected bool
		}{
			{level: 0, config: Config{Level: 0}, expected: true},  // ERROR
			{level: 1, config: Config{Level: 1}, expected: true},  // WARN
			{level: 2, config: Config{Level: 1}, expected: false}, // INFO vs WARN
			{level: 3, config: Config{Level: 2}, expected: false}, // DEBUG vs INFO
		}

		for _, tt := range tests {
			Initialize(tt.config)
			if got := V(tt.level); got != tt.expected {
				t.Errorf("V(%d) with level %d = %v; want %v",
					tt.level, tt.config.Level, got, tt.expected)
			}
		}
	})
}
