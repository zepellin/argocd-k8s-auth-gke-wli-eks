package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
)

const (
	eksClusterIdHeader = "x-k8s-aws-id"
	// The sts GetCallerIdentity request is valid for 15 minutes regardless of this parameters value after it has been
	// signed, but we set this unused parameter to 60 for legacy reasons (we check for a value between 0 and 60 on the
	// server side in 0.3.0 or earlier).  IT IS IGNORED.  If we can get STS to support x-amz-expires, then we should
	// set this parameter to the actual expiration, and make it configurable.
	requestPresignParam = 60
	// eksClusterName      = "seom"
	// The actual token expiration (presigned STS urls are valid for 15 minutes after timestamp in x-amz-date).
	presignedURLExpiration = 15 * time.Minute
	tokenV1Prefix          = "k8s-aws-v1."
)

func gcpMetadataClient() *metadata.Client {
	c := metadata.NewClient(&http.Client{Timeout: 1 * time.Second, Transport: userAgentTransport{
		userAgent: "my-user-agent",
		base:      http.DefaultTransport,
	}})
	return c
}

// This example demonstrates how to use your own transport when using this package.
func main() {
	awsAssumeRoleArn := os.Args[1]
	eksClusterName := os.Args[2]
	stsRegion := os.Args[3]

	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Retrieve information from GCP API
	c := gcpMetadataClient()

	projectId, err := c.ProjectID()
	if err != nil {
		logger.Error("Couldn't connect to GCP metadata server")
		os.Exit(1)
	}

	hostname, err := c.Hostname()
	if err != nil {
		logger.Error("Couldn't connect to GCP metadata server")
		os.Exit(1)
	}

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?format=standard&audience=gcp", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	response, _ := client.Do(req)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Error("Couldn't connect to GCP metadata server %s", err)
		os.Exit(1)
	}
	response.Body.Close()

	f, err := os.CreateTemp("", "token")

	if err != nil {
		logger.Error("%s", err)
		os.Exit(1)
	}

	defer f.Close()

	_, err2 := f.Write(body)

	if err2 != nil {
		logger.Error("%s", err2)
		os.Exit(1)
	}

	// Set custom session identifier
	sessionIdentifier := fmt.Sprintf("%s-%s", projectId, hostname)[:32]

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(stsRegion))
	if err != nil {
		logger.Error("failed to load default AWS config, %s" + err.Error())
		os.Exit(1)
	}

	stsAssumeClient := sts.NewFromConfig(cfg)

	awsCredsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
		stsAssumeClient,
		awsAssumeRoleArn,
		stscreds.IdentityTokenFile(f.Name()),
		func(o *stscreds.WebIdentityRoleOptions) {
			o.RoleSessionName = sessionIdentifier
		}),
	)

	defer os.Remove(f.Name())

	awsCredentials, err := awsCredsCache.Retrieve(ctx)
	if err != nil {
		logger.Error("Couldn't retrieve AWS credentials %s", err)
		os.Exit(1)
	}

	cfg2, err := config.LoadDefaultConfig(ctx, config.WithRegion(stsRegion),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: awsCredentials,
		}),
	)
	if err != nil {
		logger.Error("Couldn't load AWS config using retrieved credentials %s", err)
		os.Exit(1)
	}

	stsClient := sts.NewFromConfig(cfg2)

	presignclient := sts.NewPresignClient(stsClient)
	presignedURLString, err := presignclient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(opt *sts.PresignOptions) {
		opt.Presigner = newCustomHTTPPresignerV4(opt.Presigner, map[string]string{
			eksClusterIdHeader: eksClusterName,
			"X-Amz-Expires":    "60",
		})
	})

	token := tokenV1Prefix + base64.RawURLEncoding.EncodeToString([]byte(presignedURLString.URL))
	// Set token expiration to 1 minute before the presigned URL expires for some cushion
	tokenExpiration := time.Now().Local().Add(presignedURLExpiration - 1*time.Minute)
	_, _ = fmt.Fprint(os.Stdout, formatJSON(token, tokenExpiration))
}

// userAgentTransport sets the User-Agent header before calling base.
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

// RoundTrip implements the http.RoundTripper interface.
func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

func formatJSON(token string, expiration time.Time) string {
	expirationTimestamp := metav1.NewTime(expiration)
	execInput := &clientauthv1beta1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Kind:       "ExecCredential",
		},
		Status: &clientauthv1beta1.ExecCredentialStatus{
			ExpirationTimestamp: &expirationTimestamp,
			Token:               token,
		},
	}
	enc, _ := json.Marshal(execInput)
	return string(enc)
}

type customHTTPPresignerV4 struct {
	client  sts.HTTPPresignerV4
	headers map[string]string
}

func newCustomHTTPPresignerV4(client sts.HTTPPresignerV4, headers map[string]string) sts.HTTPPresignerV4 {
	return &customHTTPPresignerV4{
		client:  client,
		headers: headers,
	}
}

func (p *customHTTPPresignerV4) PresignHTTP(
	ctx context.Context, credentials aws.Credentials, r *http.Request,
	payloadHash string, service string, region string, signingTime time.Time,
	optFns ...func(*v4.SignerOptions),
) (url string, signedHeader http.Header, err error) {
	for key, val := range p.headers {
		r.Header.Add(key, val)
	}
	return p.client.PresignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
}
