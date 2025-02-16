package config

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config == nil {
		t.Fatal("NewConfig() returned nil")
	}

	// Test default values
	tests := []struct {
		name     string
		got      interface{}
		want     interface{}
		fieldStr string
	}{
		{"LogVerbosity", config.LogVerbosity, 0, "LogVerbosity"},
		{"LogToFile", config.LogToFile, "", "LogToFile"},
		{"STSRegion", config.STSRegion, DefaultSTSRegion, "STSRegion"},
		{"TokenExpiration", config.TokenExpiration, DefaultTokenExpiryMinutes * time.Minute, "TokenExpiration"},
		{"HTTPTimeout", config.HTTPTimeout, DefaultHTTPTimeout, "HTTPTimeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.fieldStr, tt.got, tt.want)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid configuration",
			config: Config{
				LogVerbosity:   1,
				AWSRoleARN:     "arn:aws:iam::123456789012:role/test-role",
				EKSClusterName: "test-cluster",
				STSRegion:      DefaultSTSRegion,
			},
			wantError: false,
		},
		{
			name: "invalid log verbosity",
			config: Config{
				LogVerbosity:   6,
				AWSRoleARN:     "arn:aws:iam::123456789012:role/test-role",
				EKSClusterName: "test-cluster",
			},
			wantError: true,
		},
		{
			name: "missing AWS role ARN",
			config: Config{
				LogVerbosity:   1,
				EKSClusterName: "test-cluster",
			},
			wantError: true,
		},
		{
			name: "missing EKS cluster name",
			config: Config{
				LogVerbosity: 1,
				AWSRoleARN:   "arn:aws:iam::123456789012:role/test-role",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if (err != nil) != tt.wantError {
				t.Errorf("validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestLoadFromFlags(t *testing.T) {
	// Save original command line arguments
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()

	tests := []struct {
		name      string
		args      []string
		wantError bool
		validate  func(*testing.T, *Config)
	}{
		{
			name: "valid flags",
			args: []string{
				"cmd",
				"-rolearn", "arn:aws:iam::123456789012:role/test-role",
				"-cluster", "test-cluster",
				"-stsregion", "us-west-2",
				"-v", "3",
				"-hybrid",
				"-cache",
			},
			wantError: false,
			validate: func(t *testing.T, c *Config) {
				if c.AWSRoleARN != "arn:aws:iam::123456789012:role/test-role" {
					t.Errorf("unexpected role ARN: got %v, want %v", c.AWSRoleARN, "arn:aws:iam::123456789012:role/test-role")
				}
				if c.EKSClusterName != "test-cluster" {
					t.Errorf("unexpected cluster name: got %v, want %v", c.EKSClusterName, "test-cluster")
				}
				if c.STSRegion != "us-west-2" {
					t.Errorf("unexpected STS region: got %v, want %v", c.STSRegion, "us-west-2")
				}
				if c.LogVerbosity != 3 {
					t.Errorf("unexpected log verbosity: got %v, want %v", c.LogVerbosity, 3)
				}
				if !c.HybridMode {
					t.Error("hybrid mode should be enabled")
				}
				if !c.Cache {
					t.Error("cache should be enabled")
				}
			},
		},
		{
			name: "missing required flags",
			args: []string{
				"cmd",
				"-v", "3",
			},
			wantError: true,
			validate:  func(t *testing.T, c *Config) {},
		},
		{
			name: "invalid log verbosity",
			args: []string{
				"cmd",
				"-rolearn", "arn:aws:iam::123456789012:role/test-role",
				"-cluster", "test-cluster",
				"-v", "6",
			},
			wantError: true,
			validate:  func(t *testing.T, c *Config) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags for each test
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ExitOnError)
			os.Args = tt.args

			config := NewConfig()
			err := config.LoadFromFlags()

			if (err != nil) != tt.wantError {
				t.Errorf("LoadFromFlags() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if err == nil {
				tt.validate(t, config)
			}
		})
	}
}
