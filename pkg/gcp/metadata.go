// Package gcp provides Google Cloud Platform related functionality
package gcp

import (
	"context"
	"fmt"
	"io"
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

// GCPMetadata implements the MetadataProvider interface
type GCPMetadata struct {
	client *metadata.Client
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
	url := fmt.Sprintf("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?format=full&audience=%s", audience)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity token request: %w", err)
	}

	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve identity token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code retrieving identity token: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity token response: %w", err)
	}

	return token, nil
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
