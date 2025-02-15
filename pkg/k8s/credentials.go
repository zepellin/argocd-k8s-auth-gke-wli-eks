// Package k8s provides Kubernetes credential handling functionality
package k8s

import (
	"encoding/base64"
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
)

const (
	// TokenV1Prefix is the prefix for v1 tokens
	TokenV1Prefix = "k8s-aws-v1."
	// TokenExpirationBuffer is the buffer time before the token actually expires
	TokenExpirationBuffer = 1 * time.Minute
)

// CredentialGenerator handles generation of Kubernetes ExecCredentials
type CredentialGenerator struct{}

// NewCredentialGenerator creates a new credential generator
func NewCredentialGenerator() *CredentialGenerator {
	return &CredentialGenerator{}
}

// GenerateExecCredential creates a Kubernetes ExecCredential from a presigned URL
func (g *CredentialGenerator) GenerateExecCredential(presignedURL string, expiration time.Time) ([]byte, error) {
	// Create the token by concatenating the prefix and base64 encoded URL
	token := TokenV1Prefix + base64.RawURLEncoding.EncodeToString([]byte(presignedURL))

	// Adjust expiration time to include buffer
	adjustedExpiration := expiration.Add(-TokenExpirationBuffer)
	expirationTimestamp := metav1.NewTime(adjustedExpiration)

	// Create the ExecCredential object
	execCred := &clientauthv1beta1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Kind:       "ExecCredential",
		},
		Status: &clientauthv1beta1.ExecCredentialStatus{
			ExpirationTimestamp: &expirationTimestamp,
			Token:               token,
		},
	}

	// Marshal to JSON
	return json.Marshal(execCred)
}
