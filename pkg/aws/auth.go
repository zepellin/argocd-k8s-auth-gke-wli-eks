// Package aws provides AWS authentication and credential management functionality
package aws

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// TokenRetriever defines the interface for retrieving identity tokens
type TokenRetriever interface {
	GetIdentityToken() ([]byte, error)
}

// STSClient wraps STS functionality for better testing
type STSClient interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// AWSAuthenticator handles AWS authentication and credential management
type AWSAuthenticator struct {
	stsClient      STSClient
	tokenRetriever TokenRetriever
	roleARN        string
	sessionName    string
	region         string
}

// NewAuthenticator creates a new AWS authenticator
func NewAuthenticator(ctx context.Context, roleARN, sessionName, region string, tokenRetriever TokenRetriever) (*AWSAuthenticator, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)

	return &AWSAuthenticator{
		stsClient:      stsClient,
		tokenRetriever: tokenRetriever,
		roleARN:        roleARN,
		sessionName:    sessionName,
		region:         region,
	}, nil
}

// GetCredentials retrieves AWS credentials using web identity federation
func (a *AWSAuthenticator) GetCredentials(ctx context.Context) (aws.Credentials, error) {
	provider := stscreds.NewWebIdentityRoleProvider(
		a.stsClient.(*sts.Client),
		a.roleARN,
		a.tokenRetriever,
		func(o *stscreds.WebIdentityRoleOptions) {
			o.RoleSessionName = a.sessionName
		},
	)

	credCache := aws.NewCredentialsCache(provider)
	creds, err := credCache.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
	}

	return creds, nil
}

// CustomPresigner adds custom headers to STS presigned URLs
type CustomPresigner struct {
	client  sts.HTTPPresignerV4
	headers map[string]string
}

// NewCustomPresigner creates a new custom presigner with specified headers
func NewCustomPresigner(client sts.HTTPPresignerV4, headers map[string]string) sts.HTTPPresignerV4 {
	return &CustomPresigner{
		client:  client,
		headers: headers,
	}
}

// PresignHTTP implements the HTTPPresignerV4 interface with custom header support
func (p *CustomPresigner) PresignHTTP(
	ctx context.Context,
	credentials aws.Credentials,
	r *http.Request,
	payloadHash string,
	service string,
	region string,
	signingTime time.Time,
	optFns ...func(*v4.SignerOptions),
) (string, http.Header, error) {
	for key, val := range p.headers {
		r.Header.Add(key, val)
	}
	return p.client.PresignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
}

// GetPresignedCallerIdentityURL generates a presigned URL for GetCallerIdentity
func (a *AWSAuthenticator) GetPresignedCallerIdentityURL(ctx context.Context, clusterName string, creds aws.Credentials) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(a.region),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: creds,
		}),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config with credentials: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)
	presignClient := sts.NewPresignClient(stsClient)

	presignedURL, err := presignClient.PresignGetCallerIdentity(ctx,
		&sts.GetCallerIdentityInput{},
		func(opt *sts.PresignOptions) {
			opt.Presigner = NewCustomPresigner(opt.Presigner, map[string]string{
				"x-k8s-aws-id":  clusterName,
				"X-Amz-Expires": "60",
			})
		})
	if err != nil {
		return "", fmt.Errorf("failed to presign GetCallerIdentity request: %w", err)
	}

	return presignedURL.URL, nil
}
