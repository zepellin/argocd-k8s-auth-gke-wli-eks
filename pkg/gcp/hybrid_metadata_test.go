package gcp

import (
	"context"
	"strings"
	"testing"
	"time"
)

// mockMetadataClient implements the MetadataClient interface for testing
type mockMetadataClient struct {
	projectID string
	hostname  string
	idToken   string
}

func (m *mockMetadataClient) ProjectID() (string, error) { return m.projectID, nil }
func (m *mockMetadataClient) Hostname() (string, error)  { return m.hostname, nil }
func (m *mockMetadataClient) Get(path string) (string, error) {
	if strings.Contains(path, "identity") {
		return m.idToken, nil
	}
	return "", nil
}

// newMockGCPMetadata creates a new GCP metadata provider with a mock client
func newMockGCPMetadata(projectID, hostname, idToken string) *GCPMetadata {
	return &GCPMetadata{
		client: &mockMetadataClient{
			projectID: projectID,
			hostname:  hostname,
			idToken:   idToken,
		},
	}
}

func TestNewHybridMetadataProvider(t *testing.T) {
	timeout := 5 * time.Second
	provider := NewHybridMetadataProvider(timeout)

	if provider == nil {
		t.Fatal("NewHybridMetadataProvider returned nil")
	}

	hybrid, ok := provider.(*HybridMetadata)
	if !ok {
		t.Fatal("Provider is not of type *HybridMetadata")
	}

	if hybrid.gcpMetadata == nil {
		t.Fatal("GCPMetadata client is nil")
	}
}

func TestHybridMetadata_ProjectID(t *testing.T) {
	tests := []struct {
		name     string
		isOnGCP  bool
		wantErr  bool
		validate func(*testing.T, string)
	}{
		{
			name:    "on GCP",
			isOnGCP: true,
			wantErr: false,
			validate: func(t *testing.T, projectID string) {
				if projectID != "test-project" {
					t.Errorf("ProjectID = %v, want %v", projectID, "test-project")
				}
			},
		},
		{
			name:    "not on GCP",
			isOnGCP: false,
			wantErr: false,
			validate: func(t *testing.T, projectID string) {
				if !strings.HasPrefix(projectID, "external-project-") {
					t.Errorf("ProjectID does not have expected prefix, got: %v", projectID)
				}
				if len(projectID) != len("external-project-")+8 {
					t.Errorf("ProjectID has unexpected length: %v", len(projectID))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &HybridMetadata{
				gcpMetadata: newMockGCPMetadata("test-project", "test-instance", "test-token"),
				isOnGCP:     tt.isOnGCP,
			}

			projectID, err := provider.ProjectID(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ProjectID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.validate(t, projectID)
		})
	}
}

func TestHybridMetadata_Hostname(t *testing.T) {
	tests := []struct {
		name     string
		isOnGCP  bool
		wantErr  bool
		validate func(*testing.T, string)
	}{
		{
			name:    "on GCP",
			isOnGCP: true,
			wantErr: false,
			validate: func(t *testing.T, hostname string) {
				if hostname != "test-instance" {
					t.Errorf("Hostname = %v, want %v", hostname, "test-instance")
				}
			},
		},
		// {
		// 	name:    "not on GCP",
		// 	isOnGCP: false,
		// 	wantErr: false,
		// 	validate: func(t *testing.T, hostname string) {
		// 		if !strings.HasPrefix(hostname, "external-host-") && !strings.Contains(hostname, ".") {
		// 			t.Errorf("Invalid hostname format: %v", hostname)
		// 		}
		// 	},
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &HybridMetadata{
				gcpMetadata: newMockGCPMetadata("test-project", "test-instance", "test-token"),
				isOnGCP:     tt.isOnGCP,
			}

			hostname, err := provider.Hostname(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Hostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.validate(t, hostname)
		})
	}
}

func TestHybridMetadata_GetIdentityToken(t *testing.T) {
	tests := []struct {
		name      string
		isOnGCP   bool
		audience  string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "on GCP",
			isOnGCP:   true,
			audience:  "test-audience",
			wantToken: "test-token",
			wantErr:   false,
		},
		// {
		// 	name:      "not on GCP",
		// 	isOnGCP:   false,
		// 	audience:  "test-audience",
		// 	wantToken: "",
		// 	wantErr:   true, // Will fail without real credentials
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &HybridMetadata{
				gcpMetadata: newMockGCPMetadata("test-project", "test-instance", tt.wantToken),
				isOnGCP:     tt.isOnGCP,
			}

			token, err := provider.GetIdentityToken(context.Background(), tt.audience)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetIdentityToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && string(token) != tt.wantToken {
				t.Errorf("GetIdentityToken() = %v, want %v", string(token), tt.wantToken)
			}
		})
	}
}

func TestHybridMetadata_CreateSessionIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		isOnGCP  bool
		wantErr  bool
		validate func(*testing.T, string)
	}{
		{
			name:    "on GCP",
			isOnGCP: true,
			wantErr: false,
			validate: func(t *testing.T, sessionID string) {
				expected := "test-project-test-instance"
				if sessionID != expected {
					t.Errorf("CreateSessionIdentifier() = %v, want %v", sessionID, expected)
				}
			},
		},
		{
			name:    "not on GCP with long names",
			isOnGCP: false,
			wantErr: false,
			validate: func(t *testing.T, sessionID string) {
				if len(sessionID) > 32 {
					t.Errorf("Session identifier exceeds 32 characters: %v", len(sessionID))
				}
				if !strings.Contains(sessionID, "-") {
					t.Error("Session identifier should contain a hyphen")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &HybridMetadata{
				gcpMetadata: newMockGCPMetadata("test-project", "test-instance", "test-token"),
				isOnGCP:     tt.isOnGCP,
			}

			sessionID, err := provider.CreateSessionIdentifier(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSessionIdentifier() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			tt.validate(t, sessionID)
		})
	}
}

func TestGenerateRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"zero length", 0},
		{"small string", 8},
		{"medium string", 16},
		{"large string", 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateRandomString(tt.length)

			if len(result) != tt.length {
				t.Errorf("generateRandomString() length = %v, want %v", len(result), tt.length)
			}

			// Verify characters are valid
			for _, c := range result {
				if !strings.ContainsRune(letterBytes, c) {
					t.Errorf("generateRandomString() contains invalid character: %c", c)
				}
			}
		})
	}
}
