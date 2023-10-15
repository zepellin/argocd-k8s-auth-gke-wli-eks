package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
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
	eksClusterIdHeader = "x-k8s-aws-id" // Header name identifying EKS cluser in STS getCallerIdentity call
	// The sts GetCallerIdentity request is valid for 15 minutes regardless of this parameters value after it has been
	// signed, but we set this unused parameter to 60 for legacy reasons (we check for a value between 0 and 60 on the
	// server side in 0.3.0 or earlier).  IT IS IGNORED.  If we can get STS to support x-amz-expires, then we should
	// set this parameter to the actual expiration, and make it configurable.
	requestPresignParam    = 60
	presignedURLExpiration = 15 * time.Minute // The actual token expiration (presigned STS urls are valid for 15 minutes after timestamp in x-amz-date).
	tokenV1Prefix          = "k8s-aws-v1."    // Prefix of a token in client.authentication.k8s.io/v1beta1 ExecCredential
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// Creates GCP metadata client
func gcpMetadataClient() *metadata.Client {
	c := metadata.NewClient(&http.Client{Timeout: 1 * time.Second})
	return c
}

// Constucts AWs session identifier from GCP metadata infrmation.
// This implementation uses concentration of  GCP project ID and machine hostname
func createSessionIdentifier(c *metadata.Client) (string, error) {
	projectId, err := c.ProjectID()
	if err != nil {
		logger.Error("Couldn't fetch ProjectId from GCP metadata server")
		return "", err
	}

	hostname, err := c.Hostname()
	if err != nil {
		logger.Error("Couldn't fetch Hostname from GCP metadata server")
		return "", err
	}

	return (fmt.Sprintf("%s-%s", projectId, hostname)[:32]), nil
}

// Retrieves GCE identity token (JWT) and retuens [customIdentityTokenRetriever] instance
// containing the token. This is to be then used in [stscreds.NewWebIdentityRoleProvider]
// function.
func gcpRetrieveGCEVMToken(ctx context.Context) (customIdentityTokenRetriever, error) {
	url := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?format=full&audience=gcp"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return customIdentityTokenRetriever{token: nil}, fmt.Errorf("http.NewRequest: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return customIdentityTokenRetriever{token: nil}, fmt.Errorf("client.Do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return customIdentityTokenRetriever{token: nil}, fmt.Errorf("status code %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return customIdentityTokenRetriever{token: nil}, fmt.Errorf("io.ReadAll: %w", err)
	}
	gcpMetadataToken := customIdentityTokenRetriever{token: b}
	return gcpMetadataToken, nil
}

func main() {
	awsAssumeRoleArn := flag.String("rolearn", "", "AWS role ARN to assume (required)")
	eksClusterName := flag.String("cluster", "", "AWS cluster name for which we create credentials (required)")
	stsRegion := flag.String("stsregion", "us-east-1", "AWS STS region to which requests are made (optional)")

	flag.Parse()
	if *awsAssumeRoleArn == "" || *eksClusterName == "" {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	sessionIdentifier, err := createSessionIdentifier(gcpMetadataClient())
	if err != nil {
		logger.Error("Failed to create session identifier from GCP metadata, %s" + err.Error())
		os.Exit(1)
	}

	assumeRoleCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*stsRegion))
	if err != nil {
		logger.Error("failed to load default AWS config, %s" + err.Error())
		os.Exit(1)
	}

	gcpMetadataToken, err := gcpRetrieveGCEVMToken(ctx)
	if err != nil {
		logger.Error("Failed to get JWT token from GCP metadata, %s" + err.Error())
		os.Exit(1)
	}

	stsAssumeClient := sts.NewFromConfig(assumeRoleCfg)
	awsCredsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
		stsAssumeClient,
		*awsAssumeRoleArn,
		gcpMetadataToken,
		func(o *stscreds.WebIdentityRoleOptions) {
			o.RoleSessionName = sessionIdentifier
		}),
	)

	awsCredentials, err := awsCredsCache.Retrieve(ctx)
	if err != nil {
		logger.Error("Couldn't retrieve AWS credentials %s", err)
		os.Exit(1)
	}

	eksSignerCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*stsRegion),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: awsCredentials,
		}),
	)
	if err != nil {
		logger.Error("Couldn't load AWS config using retrieved credentials %s", err)
		os.Exit(1)
	}

	stsClient := sts.NewFromConfig(eksSignerCfg)

	presignclient := sts.NewPresignClient(stsClient)
	presignedURLString, err := presignclient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(opt *sts.PresignOptions) {
		opt.Presigner = newCustomHTTPPresignerV4(opt.Presigner, map[string]string{
			eksClusterIdHeader: *eksClusterName,
			"X-Amz-Expires":    "60",
		})
	})

	token := tokenV1Prefix + base64.RawURLEncoding.EncodeToString([]byte(presignedURLString.URL))
	// Set token expiration to 1 minute before the presigned URL expires for some cushion
	tokenExpiration := time.Now().Local().Add(presignedURLExpiration - 1*time.Minute)
	_, _ = fmt.Fprint(os.Stdout, formatJSON(token, tokenExpiration))
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

type customIdentityTokenRetriever struct {
	token []byte
}

func (obj customIdentityTokenRetriever) GetIdentityToken() ([]byte, error) {
	return obj.token, nil
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
