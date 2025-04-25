package aws

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	smithyendpoints "github.com/aws/smithy-go/endpoints"

	"argocd-k8s-auth-gke-wli-eks/pkg/config"
)

// TokenRetriever interface to retrieve identity token
type TokenRetriever interface {
	GetIdentityToken() ([]byte, error)
}

// webIdentityTokenProvider implements stscreds.WebIdentityRoleProvider interface
type webIdentityTokenProvider struct {
	token []byte
}

func (p *webIdentityTokenProvider) GetIdentityToken() ([]byte, error) {
	return p.token, nil
}

type resolverV2 struct {
	url *string
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

func (r *resolverV2) ResolveEndpoint(ctx context.Context, params sts.EndpointParameters) (
	smithyendpoints.Endpoint, error,
) {
	// set the endpoint to the provided URL if it's not nil
	if r.url != nil {
		params.Endpoint = aws.String(*r.url)
	}

	// delegate back to the default v2 resolver otherwise
	return sts.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, params)
}

// Authenticator handles AWS authentication
type Authenticator struct {
	roleARN        string
	sessionID      string
	stsRegion      string
	tokenRetriever TokenRetriever
	awsEndpointUrl string
}

// NewAuthenticator creates a new AWS authenticator
func NewAuthenticator(ctx context.Context, roleARN, sessionID, stsRegion string, tokenRetriever TokenRetriever, awsEndpointUrl string) (*Authenticator, error) {
	if roleARN == "" {
		return nil, fmt.Errorf("AWS role ARN is required")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	if stsRegion == "" {
		return nil, fmt.Errorf("AWS STS region is required")
	}
	if tokenRetriever == nil {
		return nil, fmt.Errorf("token retriever is required")
	}
	if awsEndpointUrl == "" {
		awsEndpointUrl = fmt.Sprintf("https://sts.%s.amazonaws.com", stsRegion)
	}

	return &Authenticator{
		roleARN:        roleARN,
		sessionID:      sessionID,
		stsRegion:      stsRegion,
		tokenRetriever: tokenRetriever,
		awsEndpointUrl: awsEndpointUrl,
	}, nil
}

// GetCredentials retrieves AWS credentials
func (a *Authenticator) GetCredentials(ctx context.Context) (*types.Credentials, error) {
	cfg, err := a.getAWSConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS config: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	return &types.Credentials{
		AccessKeyId:     aws.String(creds.AccessKeyID),
		SecretAccessKey: aws.String(creds.SecretAccessKey),
		SessionToken:    aws.String(creds.SessionToken),
	}, nil
}

// GetPresignedCallerIdentityURL gets a presigned URL for EKS cluster authentication
func (a *Authenticator) GetPresignedCallerIdentityURL(ctx context.Context, clusterName string, creds *types.Credentials, expiration time.Duration) (string, error) {
	cfg, err := a.getAWSConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get AWS config: %w", err)
	}

	// Create STS client with the provided credentials
	staticCreds := credentials.NewStaticCredentialsProvider(*creds.AccessKeyId, *creds.SecretAccessKey, *creds.SessionToken)
	stsClient := sts.NewFromConfig(*cfg, func(o *sts.Options) {
		o.EndpointResolverV2 = &resolverV2{
			url: aws.String(a.awsEndpointUrl),
		}
		o.Credentials = staticCreds
	})

	// Get caller identity and generate presigned URL
	input := &sts.GetCallerIdentityInput{}
	presigner := sts.NewPresignClient(stsClient)
	output, err := presigner.PresignGetCallerIdentity(ctx, input, func(opt *sts.PresignOptions) {
		opt.Presigner = NewCustomPresigner(opt.Presigner, map[string]string{
			config.HeaderEKSClusterID: clusterName,
			config.HeaderExpires:      fmt.Sprintf("%.0f", expiration.Seconds()),
		})
	})
	if err != nil {
		return "", fmt.Errorf("failed to presign get caller identity: %w", err)
	}

	// Add cluster name to the URL as a query parameter
	presignedURL := fmt.Sprintf("%s", output.URL)

	return presignedURL, nil
}

func (a *Authenticator) getAWSConfig(ctx context.Context) (*aws.Config, error) {
	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(a.stsRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load default AWS config: %w", err)
	}

	// Retrieve identity token
	identityToken, err := a.tokenRetriever.GetIdentityToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get identity token: %w", err)
	}

	// Create STS client and assume role provider
	stsClient := sts.NewFromConfig(cfg, func(o *sts.Options) {
		o.EndpointResolverV2 = &resolverV2{
			url: aws.String(a.awsEndpointUrl),
		}
	})
	tokenProvider := &webIdentityTokenProvider{token: identityToken}
	webIdentityProvider := stscreds.NewWebIdentityRoleProvider(stsClient, a.roleARN, tokenProvider)

	// Set the credentials provider
	cfg.Credentials = aws.NewCredentialsCache(webIdentityProvider)

	return &cfg, nil
}
