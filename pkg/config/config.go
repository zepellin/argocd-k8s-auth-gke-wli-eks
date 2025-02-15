// Package config provides configuration structures and loading mechanisms
package config

import (
	"flag"
	"fmt"
	"time"
)

const (
	// DefaultSTSRegion is the default AWS STS region
	DefaultSTSRegion = "us-east-1"
	// DefaultTokenExpiryMinutes is the default token expiration time in minutes
	DefaultTokenExpiryMinutes = 15
	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 10 * time.Second
	// TokenV1Prefix is the prefix for v1 tokens
	TokenV1Prefix = "k8s-aws-v1."
	// HeaderEKSClusterID is the header name for EKS cluster identification
	HeaderEKSClusterID = "x-k8s-aws-id"
	// HeaderExpires is the header name for expiration
	HeaderExpires = "X-Amz-Expires"
	// RequestPresignParam is the presign parameter value (legacy support)
	RequestPresignParam = "60"
)

// Config holds the application configuration
type Config struct {
	// Logging configuration
	LogVerbosity int    // Verbosity level for logging (0-5)
	LogToFile    string // Path to log file (empty for stderr)

	// AWS configuration
	AWSRoleARN     string
	EKSClusterName string
	STSRegion      string

	// Token configuration
	TokenExpiration time.Duration

	// HTTP configuration
	HTTPTimeout time.Duration

	// Runtime configuration
	HybridMode bool // When true, allows running outside GCP with fallback mechanisms
}

// NewConfig creates a new configuration instance with defaults
func NewConfig() *Config {
	return &Config{
		LogVerbosity:    0,
		LogToFile:       "",
		STSRegion:       DefaultSTSRegion,
		TokenExpiration: DefaultTokenExpiryMinutes * time.Minute,
		HTTPTimeout:     DefaultHTTPTimeout,
	}
}

// LoadFromFlags loads configuration from command line flags
func (c *Config) LoadFromFlags() error {
	flag.IntVar(&c.LogVerbosity, "v", 0, "Log verbosity level (0-5)")
	flag.StringVar(&c.LogToFile, "log-file", "", "Path to log file (empty for stderr)")
	flag.StringVar(&c.AWSRoleARN, "rolearn", "", "AWS role ARN to assume (required)")
	flag.StringVar(&c.EKSClusterName, "cluster", "", "AWS cluster name for which we create credentials (required)")
	flag.StringVar(&c.STSRegion, "stsregion", DefaultSTSRegion, "AWS STS region to which requests are made (optional)")
	flag.BoolVar(&c.HybridMode, "hybrid", false, "Enable hybrid mode to run outside GCP with fallback mechanisms")

	flag.Parse()

	if err := c.validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	return nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	// Validate log verbosity
	if c.LogVerbosity < 0 || c.LogVerbosity > 5 {
		return fmt.Errorf("invalid log verbosity: %d (must be between 0 and 5)", c.LogVerbosity)
	}

	if c.AWSRoleARN == "" {
		return fmt.Errorf("AWS role ARN is required")
	}
	if c.EKSClusterName == "" {
		return fmt.Errorf("EKS cluster name is required")
	}
	return nil
}
