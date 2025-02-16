// Package gcp provides Google Cloud Platform related functionality
package gcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/compute/metadata"
)

// MetadataProvider defines the interface for GCP metadata operations
type MetadataProvider interface {
	ProjectID(ctx context.Context) (string, error)
	Hostname(ctx context.Context) (string, error)
	GetIdentityToken(ctx context.Context, audience string) ([]byte, error)
	CreateSessionIdentifier(ctx context.Context) (string, error)
}

// MetadataClient defines the interface for metadata client operations
type MetadataClient interface {
	ProjectID() (string, error)
	Hostname() (string, error)
	Get(string) (string, error)
}

// GCPMetadata implements the MetadataProvider interface
type GCPMetadata struct {
	client MetadataClient
}

// NewMetadataProvider creates a new GCP metadata provider
func NewMetadataProvider(timeout time.Duration) MetadataProvider {
	return &GCPMetadata{
		client: metadata.NewClient(&http.Client{Timeout: timeout}),
	}
}

// ProjectID retrieves the GCP project ID from metadata
func (g *GCPMetadata) ProjectID(ctx context.Context) (string, error) {
	projectID, err := g.client.ProjectID()
	if err != nil {
		return "", fmt.Errorf("failed to fetch ProjectID from GCP metadata: %w", err)
	}
	return projectID, nil
}

// Hostname retrieves the instance hostname from metadata
func (g *GCPMetadata) Hostname(ctx context.Context) (string, error) {
	hostname, err := g.client.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to fetch Hostname from GCP metadata: %w", err)
	}
	return hostname, nil
}

// GetIdentityToken retrieves a GCP identity token
func (g *GCPMetadata) GetIdentityToken(ctx context.Context, audience string) ([]byte, error) {
	token, err := g.client.Get("instance/service-accounts/default/identity?format=full&audience=" + audience)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve identity token: %w", err)
	}
	return []byte(token), nil
}

// CreateSessionIdentifier creates a unique session identifier from GCP metadata
func (g *GCPMetadata) CreateSessionIdentifier(ctx context.Context) (string, error) {
	projectID, err := g.ProjectID(ctx)
	if err != nil {
		return "", err
	}

	hostname, err := g.Hostname(ctx)
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
