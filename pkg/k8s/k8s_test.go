package k8s

import (
	"encoding/json"
	"testing"
	"time"

	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
)

func TestGenerateExecCredential(t *testing.T) {
	generator := NewCredentialGenerator()
	presignedURL := "https://example.com/presigned-url"
	expiration := time.Now().Add(time.Hour)

	data, err := generator.GenerateExecCredential(presignedURL, expiration)
	if err != nil {
		t.Fatalf("Error generating ExecCredential: %v", err)
	}

	var execCred clientauthv1beta1.ExecCredential
	err = json.Unmarshal(data, &execCred)
	if err != nil {
		t.Fatalf("Error unmarshaling ExecCredential: %v", err)
	}

	if execCred.APIVersion != "client.authentication.k8s.io/v1beta1" {
		t.Errorf("Expected APIVersion to be client.authentication.k8s.io/v1beta1, but got %s", execCred.APIVersion)
	}

	if execCred.Kind != "ExecCredential" {
		t.Errorf("Expected Kind to be ExecCredential, but got %s", execCred.Kind)
	}

	if execCred.Status == nil {
		t.Fatalf("Expected Status to not be nil")
	}

	if execCred.Status.Token == "" {
		t.Errorf("Expected Token to not be empty")
	}

	expectedPrefix := TokenV1Prefix
	if len(execCred.Status.Token) <= len(expectedPrefix) || execCred.Status.Token[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected token to start with %s, but got %s", expectedPrefix, execCred.Status.Token[:len(expectedPrefix)])
	}

	if execCred.Status.ExpirationTimestamp == nil {
		t.Fatalf("Expected ExpirationTimestamp to not be nil")
	}

	// Check if the expiration timestamp is within a reasonable range
	expectedExpiration := expiration.Add(-TokenExpirationBuffer)
	timeDiff := expectedExpiration.Sub(execCred.Status.ExpirationTimestamp.Time)
	if timeDiff > time.Minute {
		t.Errorf("Expiration timestamp is too far off from the expected time")
	}
}
