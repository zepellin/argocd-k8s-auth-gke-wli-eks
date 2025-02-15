// Package cache provides credential caching functionality
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"argocd-k8s-auth-gke-wli-eks/pkg/logger"
)

const (
	// minValidityPeriod is the minimum time remaining before a cached credential is considered invalid
	minValidityPeriod = 5 * time.Minute
)

// CacheKey represents the unique identifier for cached credentials
type CacheKey struct {
	AWSRoleARN     string `json:"aws_role_arn"`
	EKSClusterName string `json:"eks_cluster_name"`
	STSRegion      string `json:"sts_region"`
}

// CacheEntry represents a cached credential
type CacheEntry struct {
	ExecCredential []byte    `json:"exec_credential"`
	ExpirationTime time.Time `json:"expiration_time"`
}

// Cache handles credential caching operations
type Cache struct {
	cacheDir string
}

// NewCache creates a new cache instance
func NewCache() (*Cache, error) {
	var cacheDir string
	var err error

	// Try user home directory first
	homeDir, err := os.UserHomeDir()
	if err == nil {
		cacheDir = filepath.Join(homeDir, ".kube", "cache", "argocd-k8s-auth-gke-wli-eks")
		if err := os.MkdirAll(cacheDir, 0700); err == nil {
			logger.Debug("using cache directory: %s", cacheDir)
			return &Cache{cacheDir: cacheDir}, nil
		}
		logger.Warning("failed to create cache directory in home directory: %v", err)
	} else {
		logger.Warning("failed to get user home directory: %v", err)
	}

	// If home directory fails, try system temporary directory
	cacheDir, err = os.UserCacheDir()
	if err == nil {
		cacheDir = filepath.Join(cacheDir, "argocd-k8s-auth-gke-wli-eks")
		if err := os.MkdirAll(cacheDir, 0700); err == nil {
			logger.Debug("using cache directory: %s", cacheDir)
			return &Cache{cacheDir: cacheDir}, nil
		}
		logger.Warning("failed to create cache directory in user cache directory: %v", err)

	} else {
		logger.Warning("failed to get user cache directory: %v", err)
	}

	// If both fail, try system temporary directory
	cacheDir = os.TempDir()
	cacheDir = filepath.Join(cacheDir, "argocd-k8s-auth-gke-wli-eks")
	if err := os.MkdirAll(cacheDir, 0700); err == nil {
		logger.Debug("using cache directory: %s", cacheDir)
		return &Cache{cacheDir: cacheDir}, nil
	}
	logger.Warning("failed to create cache directory in temporary directory: %v", err)

	return nil, fmt.Errorf("failed to create cache directory in any known location")
}

// Get retrieves cached credentials if they exist and are still valid
func (c *Cache) Get(key CacheKey) ([]byte, bool) {
	cacheFile := c.getCacheFilePath(key)

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		logger.Debug("no cache file found at %s", cacheFile)
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		logger.Debug("failed to unmarshal cache entry: %v", err)
		return nil, false
	}

	// Check if the cached credential is still valid (has more than minValidityPeriod until expiration)
	if time.Until(entry.ExpirationTime) < minValidityPeriod {
		logger.Debug("cached credential is expired or will expire soon")
		return nil, false
	}

	logger.Debug("using cached credential (expires in %v)", time.Until(entry.ExpirationTime))
	return entry.ExecCredential, true
}

// Put stores credentials in the cache
func (c *Cache) Put(key CacheKey, execCredential []byte, expirationTime time.Time) error {
	entry := CacheEntry{
		ExecCredential: execCredential,
		ExpirationTime: expirationTime,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	cacheFile := c.getCacheFilePath(key)
	if err := os.WriteFile(cacheFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	logger.Debug("stored credential in cache (expires at %v)", expirationTime)
	return nil
}

// getCacheFilePath returns the path to the cache file for the given key
func (c *Cache) getCacheFilePath(key CacheKey) string {
	// Create a unique filename based on the key components
	// Replace special characters with underscores to ensure valid filename
	sanitizedRole := strings.ReplaceAll(strings.ReplaceAll(key.AWSRoleARN, "/", "_"), ":", "_")
	sanitizedCluster := strings.ReplaceAll(key.EKSClusterName, "/", "_")
	sanitizedRegion := strings.ReplaceAll(key.STSRegion, "/", "_")
	filename := fmt.Sprintf("%s_%s_%s.json", sanitizedRole, sanitizedCluster, sanitizedRegion)
	return filepath.Join(c.cacheDir, filename)
}
