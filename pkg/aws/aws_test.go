package aws

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var motoEndpoint string

func setupMotoContainer(t *testing.T) (testcontainers.Container, string) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "motoserver/moto:latest",
		ExposedPorts: []string{"5000/tcp"},
		WaitingFor:   wait.ForListeningPort("5000/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5000")
	if err != nil {
		t.Fatalf("failed to get container external port: %s", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %s", err)
	}

	return container, fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
}

func TestMain(m *testing.M) {
	// Setup
	container, endpoint := setupMotoContainer(&testing.T{})
	motoEndpoint = endpoint

	// Run tests
	code := m.Run()

	// Cleanup
	if err := container.Terminate(context.Background()); err != nil {
		fmt.Printf("failed to terminate container: %s\n", err)
	}

	os.Exit(code)
}

type mockTokenRetriever struct {
	token []byte
	err   error
}

func (m *mockTokenRetriever) GetIdentityToken() ([]byte, error) {
	return m.token, m.err
}

func TestNewAuthenticator(t *testing.T) {
	tests := []struct {
		name           string
		roleARN        string
		sessionID      string
		stsRegion      string
		tokenRetriever TokenRetriever
		awsEndpointUrl string
		wantErr        bool
		errMsg         string
	}{
		{
			name:           "valid input",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "us-east-1",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        false,
		},
		{
			name:           "empty role ARN",
			roleARN:        "",
			sessionID:      "test-session",
			stsRegion:      "us-east-1",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        true,
			errMsg:         "AWS role ARN is required",
		},
		{
			name:           "empty session ID",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "",
			stsRegion:      "us-east-1",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        true,
			errMsg:         "session ID is required",
		},
		{
			name:           "empty STS region",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        true,
			errMsg:         "AWS STS region is required",
		},
		{
			name:           "nil token retriever",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "us-east-1",
			tokenRetriever: nil,
			awsEndpointUrl: motoEndpoint,
			wantErr:        true,
			errMsg:         "token retriever is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(context.Background(), tt.roleARN, tt.sessionID, tt.stsRegion, tt.tokenRetriever, tt.awsEndpointUrl)
			if tt.wantErr {
				if err == nil {
					t.Error("NewAuthenticator() expected error, got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("NewAuthenticator() error = %v, want %v", err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("NewAuthenticator() unexpected error: %v", err)
				return
			}
			if auth == nil {
				t.Error("NewAuthenticator() returned nil authenticator")
				return
			}
			if auth.roleARN != tt.roleARN {
				t.Errorf("NewAuthenticator() roleARN = %v, want %v", auth.roleARN, tt.roleARN)
			}
			if auth.sessionID != tt.sessionID {
				t.Errorf("NewAuthenticator() sessionID = %v, want %v", auth.sessionID, tt.sessionID)
			}
			if auth.stsRegion != tt.stsRegion {
				t.Errorf("NewAuthenticator() stsRegion = %v, want %v", auth.stsRegion, tt.stsRegion)
			}
		})
	}
}

func TestGetCredentials(t *testing.T) {
	tests := []struct {
		name           string
		roleARN        string
		sessionID      string
		stsRegion      string
		tokenRetriever TokenRetriever
		awsEndpointUrl string
		wantErr        bool
	}{
		{
			name:           "successful credentials retrieval",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "eu-south-2",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        false,
		},
		{
			name:           "token retriever error",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "eu-south-2",
			tokenRetriever: &mockTokenRetriever{token: nil, err: fmt.Errorf("token error")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(context.Background(), tt.roleARN, tt.sessionID, tt.stsRegion, tt.tokenRetriever, tt.awsEndpointUrl)
			if err != nil {
				t.Fatalf("NewAuthenticator() unexpected error: %v", err)
			}

			creds, err := auth.GetCredentials(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Error("GetCredentials() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("GetCredentials() unexpected error: %v", err)
				return
			}
			if creds == nil {
				t.Error("GetCredentials() returned nil credentials")
				return
			}
			if creds.AccessKeyId == nil || *creds.AccessKeyId == "" {
				t.Error("GetCredentials() AccessKeyId is empty")
			}
			if creds.SecretAccessKey == nil || *creds.SecretAccessKey == "" {
				t.Error("GetCredentials() SecretAccessKey is empty")
			}
			if creds.SessionToken == nil || *creds.SessionToken == "" {
				t.Error("GetCredentials() SessionToken is empty")
			}
		})
	}
}

func TestGetPresignedCallerIdentityURL(t *testing.T) {
	tests := []struct {
		name           string
		roleARN        string
		sessionID      string
		stsRegion      string
		clusterName    string
		tokenRetriever TokenRetriever
		awsEndpointUrl string
		wantErr        bool
	}{
		{
			name:           "successful URL generation",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "us-east-1",
			clusterName:    "test-cluster",
			tokenRetriever: &mockTokenRetriever{token: []byte("test-token")},
			awsEndpointUrl: motoEndpoint,
			wantErr:        false,
		},
		{
			name:           "token retriever error",
			roleARN:        "arn:aws:iam::123456789012:role/test-role",
			sessionID:      "test-session",
			stsRegion:      "us-east-1",
			clusterName:    "test-cluster",
			tokenRetriever: &mockTokenRetriever{token: nil, err: fmt.Errorf("token error")},
			awsEndpointUrl: "http://127.0.0.1:50001",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewAuthenticator(context.Background(), tt.roleARN, tt.sessionID, tt.stsRegion, tt.tokenRetriever, tt.awsEndpointUrl)
			if err != nil {
				t.Fatalf("NewAuthenticator() unexpected error: %v", err)
			}

			creds, err := auth.GetCredentials(context.Background())
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("GetCredentials() unexpected error: %v", err)
				}
				return
			}

			url, err := auth.GetPresignedCallerIdentityURL(context.Background(), tt.clusterName, creds, time.Hour)
			if tt.wantErr {
				if err == nil {
					t.Error("GetPresignedCallerIdentityURL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("GetPresignedCallerIdentityURL() unexpected error: %v", err)
				return
			}
			if url == "" {
				t.Error("GetPresignedCallerIdentityURL() returned empty URL")
			}
		})
	}
}
