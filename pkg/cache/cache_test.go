package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"argocd-k8s-auth-gke-wli-eks/pkg/logger"
)

func init() {
	// Initialize logger with debug level for tests
	if err := logger.Initialize(logger.Config{Level: 2, Verbosity: 1}); err != nil {
		panic(err)
	}
}

func TestNewCache(t *testing.T) {
	tests := []struct {
		name    string
		tempDir string
		wantErr bool
	}{
		{
			name:    "successful cache creation",
			tempDir: t.TempDir(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set temp dir for testing
			os.Setenv("HOME", tt.tempDir)

			cache, err := NewCache()
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCache() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if cache == nil && !tt.wantErr {
				t.Error("NewCache() returned nil cache without error")
			}
		})
	}
}

func TestCacheOperations(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	testKey := CacheKey{
		AWSRoleARN:     "arn:aws:iam::123456789012:role/test-role",
		EKSClusterName: "test-cluster",
		STSRegion:      "us-east-1",
	}

	testData := []byte(`{"test": "data"}`)
	futureTime := time.Now().Add(30 * time.Minute)

	// Test Put operation
	t.Run("Put", func(t *testing.T) {
		err := cache.Put(testKey, testData, futureTime)
		if err != nil {
			t.Errorf("Put() error = %v", err)
		}

		// Verify file exists
		path := cache.getCacheFilePath(testKey)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Cache file was not created at %s", path)
		}
	})

	// Test Get operation with valid cache
	t.Run("Get valid cache", func(t *testing.T) {
		data, exists := cache.Get(testKey)
		if !exists {
			t.Error("Get() should return true for existing cache")
		}
		if string(data) != string(testData) {
			t.Errorf("Get() data = %s, want %s", string(data), string(testData))
		}
	})

	// Test Get operation with expired cache
	t.Run("Get expired cache", func(t *testing.T) {
		expiredKey := CacheKey{
			AWSRoleARN:     "arn:aws:iam::123456789012:role/expired",
			EKSClusterName: "expired-cluster",
			STSRegion:      "us-east-1",
		}
		expiredTime := time.Now().Add(-10 * time.Minute)

		err := cache.Put(expiredKey, testData, expiredTime)
		if err != nil {
			t.Fatalf("Failed to put expired cache: %v", err)
		}

		data, exists := cache.Get(expiredKey)
		if exists {
			t.Error("Get() should return false for expired cache")
		}
		if data != nil {
			t.Errorf("Get() should return nil data for expired cache")
		}
	})

	// Test file path sanitization
	t.Run("File path sanitization", func(t *testing.T) {
		specialKey := CacheKey{
			AWSRoleARN:     "arn:aws:iam::123456789012:role/special/chars:test",
			EKSClusterName: "cluster/with/slashes",
			STSRegion:      "region/with/slashes",
		}

		err := cache.Put(specialKey, testData, futureTime)
		if err != nil {
			t.Errorf("Put() error with special characters = %v", err)
		}

		path := cache.getCacheFilePath(specialKey)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Cache file with sanitized path was not created at %s", path)
		}

		// Verify the path doesn't contain original special characters
		if filepath.Base(path) != "arn_aws_iam__123456789012_role_special_chars_test_cluster_with_slashes_region_with_slashes.json" {
			t.Errorf("File path not properly sanitized: %s", path)
		}
	})
}

func TestCacheConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key := CacheKey{
		AWSRoleARN:     "arn:aws:iam::123456789012:role/test-role",
		EKSClusterName: "test-cluster",
		STSRegion:      "us-east-1",
	}

	// Test concurrent reads and writes
	t.Run("Concurrent operations", func(t *testing.T) {
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func() {
				data := []byte(`{"test": "concurrent"}`)
				futureTime := time.Now().Add(30 * time.Minute)

				err := cache.Put(key, data, futureTime)
				if err != nil {
					t.Errorf("Concurrent Put() error = %v", err)
				}

				_, exists := cache.Get(key)
				if !exists {
					t.Error("Concurrent Get() failed to retrieve data")
				}

				done <- true
			}()
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
