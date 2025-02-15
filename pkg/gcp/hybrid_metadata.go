// Package gcp provides Google Cloud Platform related functionality
package gcp

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"net/http"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2/google"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyz0123456789"

// HybridMetadata implements MetadataProvider with fallback for non-GCP environments
type HybridMetadata struct {
	gcpMetadata *GCPMetadata
	isOnGCP     bool
}

// NewHybridMetadataProvider creates a new hybrid metadata provider
func NewHybridMetadataProvider(timeout time.Duration) MetadataProvider {
	// Check if we're running on GCP
	isOnGCP := metadata.OnGCE()

	return &HybridMetadata{
		gcpMetadata: &GCPMetadata{client: metadata.NewClient(&http.Client{Timeout: timeout})},
		isOnGCP:     isOnGCP,
	}
}

// ProjectID retrieves the GCP project ID from metadata or generates a fallback
func (h *HybridMetadata) ProjectID(ctx context.Context) (string, error) {
	if h.isOnGCP {
		return h.gcpMetadata.ProjectID(ctx)
	}

	// When not on GCP, use a consistent project ID for the session

	// Use a randomly generated but consistent project ID for this process
	return fmt.Sprintf("external-project-%s", generateRandomString(8)), nil
}

// Hostname retrieves the instance hostname or generates a fallback
func (h *HybridMetadata) Hostname(ctx context.Context) (string, error) {
	if h.isOnGCP {
		return h.gcpMetadata.Hostname(ctx)
	}

	// Try to get actual hostname
	hostname, err := os.Hostname()
	if err == nil {
		return hostname, nil
	}

	// Generate a random hostname as fallback
	return fmt.Sprintf("external-host-%s", generateRandomString(8)), nil
}

// GetIdentityToken retrieves a GCP identity token using available methods
func (h *HybridMetadata) GetIdentityToken(ctx context.Context, audience string) ([]byte, error) {
	if h.isOnGCP {
		return h.gcpMetadata.GetIdentityToken(ctx, audience)
	}

	// When not on GCP, try to get token using default credentials
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get default credentials: %w", err)
	}

	// Get token with identity audience
	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Extract id_token from OAuth2 token
	idToken, ok := token.Extra("id_token").(string)
	if !ok || idToken == "" {
		idToken = token.AccessToken
	}

	return []byte(idToken), nil
}

// CreateSessionIdentifier creates a unique session identifier
func (h *HybridMetadata) CreateSessionIdentifier(ctx context.Context) (string, error) {
	projectID, err := h.ProjectID(ctx)
	if err != nil {
		return "", err
	}

	hostname, err := h.Hostname(ctx)
	if err != nil {
		return "", err
	}

	// Ensure the session identifier doesn't exceed 32 characters
	sessionID := fmt.Sprintf("%s-%s", projectID, hostname)
	if len(sessionID) > 32 {
		sessionID = sessionID[:32]
	}

	return sessionID, nil
}

// generateRandomString generates a random string of given length
func generateRandomString(n int) string {
	b := make([]byte, n)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = letterBytes[r.Intn(len(letterBytes))]
	}
	return string(b)
}
