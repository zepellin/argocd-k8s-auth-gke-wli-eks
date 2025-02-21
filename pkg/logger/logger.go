package logger

import (
	"fmt"
	"log/slog"
	"os"
)

var (
	logger *slog.Logger
)

// Config holds logger configuration
type Config struct {
	Level     int    // Log level (0-5)
	ToFile    string // Log file path (empty for stderr)
	Verbosity int    // Verbosity level for debug logs
}

// Initialize sets up the global logger with the given configuration
func Initialize(config Config) error {
	var w *os.File
	var err error

	if config.ToFile != "" {
		w, err = os.OpenFile(config.ToFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %v", err)
		}
	} else {
		w = os.Stderr
	}

	// Convert level to slog.Level
	var level slog.Level
	switch {
	case config.Level <= 0:
		level = slog.LevelError
	case config.Level == 1:
		level = slog.LevelWarn
	case config.Level == 2:
		level = slog.LevelInfo
	default:
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(w, opts)
	logger = slog.New(handler)
	slog.SetDefault(logger)

	return nil
}

// Error logs an error message with optional format arguments
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logger.Error(msg)
}

// Errorf logs an error message with error and optional format arguments
func Errorf(err error, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logger.Error(msg, "error", err)
}

// Warning logs a warning message with optional format arguments
func Warning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logger.Warn(msg)
}

// Info logs an info message with optional format arguments
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logger.Info(msg)
}

// Infof logs an info message with optional key-value pairs
func Infof(msg string, keysAndValues ...interface{}) {
	logger.Info(msg, keysAndValues...)
}

// Debug logs a debug message if the verbosity level is high enough
func Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logger.Debug(msg)
}

// V returns true if the verbosity level is at least the requested level
func V(level int) bool {
	// Convert level to slog.Level for comparison
	var slogLevel slog.Level
	switch level {
	case 0:
		slogLevel = slog.LevelError
	case 1:
		slogLevel = slog.LevelWarn
	case 2:
		slogLevel = slog.LevelInfo
	default:
		slogLevel = slog.LevelDebug
	}
	return logger.Enabled(nil, slogLevel)
}

// Flush ensures all pending log writes are completed
func Flush() {
	// slog handles flushing automatically
}
