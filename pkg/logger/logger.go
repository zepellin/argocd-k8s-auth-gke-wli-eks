package logger

import (
	"flag"
	"fmt"

	"k8s.io/klog/v2"
)

var (
	// Global logger configuration
	logLevel  int
	logToFile string
)

// Config holds logger configuration
type Config struct {
	Level     int    // Log level (0-5)
	ToFile    string // Log file path (empty for stderr)
	Verbosity int    // Verbosity level for debug logs
}

// Initialize sets up the global logger with the given configuration
func Initialize(config Config) error {
	logLevel = config.Level
	logToFile = config.ToFile

	// Create a new flagset for klog
	fs := flag.NewFlagSet("logger", flag.ContinueOnError)
	klog.InitFlags(fs)

	// Set klog specific flags
	if err := fs.Set("v", fmt.Sprintf("%d", config.Verbosity)); err != nil {
		return fmt.Errorf("failed to set verbosity: %v", err)
	}

	if logToFile != "" {
		if err := fs.Set("log_file", logToFile); err != nil {
			return fmt.Errorf("failed to set log file: %v", err)
		}
	}

	return nil
}

// Error logs an error message with optional format arguments
func Error(format string, args ...interface{}) {
	klog.ErrorS(nil, fmt.Sprintf(format, args...))
}

// Errorf logs an error message with error and optional format arguments
func Errorf(err error, format string, args ...interface{}) {
	klog.ErrorS(err, fmt.Sprintf(format, args...))
}

// Warning logs a warning message with optional format arguments
func Warning(format string, args ...interface{}) {
	klog.Warning(fmt.Sprintf(format, args...))
}

// Info logs an info message with optional format arguments
func Info(format string, args ...interface{}) {
	klog.Info(fmt.Sprintf(format, args...))
}

// Infof logs an info message with optional key-value pairs
func Infof(msg string, keysAndValues ...interface{}) {
	klog.InfoS(msg, keysAndValues...)
}

// Debug logs a debug message if the verbosity level is high enough
func Debug(format string, args ...interface{}) {
	if klog.V(1).Enabled() {
		klog.InfoS("DEBUG", "msg", fmt.Sprintf(format, args...))
	}
}

// V returns true if the verbosity level is at least the requested level
func V(level int) bool {
	return klog.V(klog.Level(level)).Enabled()
}

// Flush ensures all pending log writes are completed
func Flush() {
	klog.Flush()
}
