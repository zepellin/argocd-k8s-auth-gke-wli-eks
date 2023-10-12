package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
)

const (
	clusterIDHeader = "x-k8s-aws-id"
	// The sts GetCallerIdentity request is valid for 15 minutes regardless of this parameters value after it has been
	// signed, but we set this unused parameter to 60 for legacy reasons (we check for a value between 0 and 60 on the
	// server side in 0.3.0 or earlier).  IT IS IGNORED.  If we can get STS to support x-amz-expires, then we should
	// set this parameter to the actual expiration, and make it configurable.
	requestPresignParam = 60
	// The actual token expiration (presigned STS urls are valid for 15 minutes after timestamp in x-amz-date).
	presignedURLExpiration = 15 * time.Minute
	v1Prefix               = "k8s-aws-v1."
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
	// Retrieve information from GCP API
	c := gcpMetadataClient()

	awsAssumeRoleArn := os.Args[1]
	eksClusterName := os.Args[2]

	projectId, err := c.ProjectID()
	if err != nil {
		log.Fatal("Couldn't connect to GCP metadata server")
	}

	hostname, err := c.Hostname()
	if err != nil {
		log.Fatal("Couldn't connect to GCP metadata server")
	}

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?format=standard&audience=gcp", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	response, _ := client.Do(req)

	body, error := io.ReadAll(response.Body)
	if error != nil {
		fmt.Println(error)
	}
	response.Body.Close()

	if err != nil {
		log.Fatal("Couldn't connect to metadata server")
	}

	// Set custom session identifier
	sessionIdentifier := fmt.Sprintf("%s-%s", projectId, hostname)[:32]

	// Here we start with AWS stuff
	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(awsAssumeRoleArn),
		RoleSessionName:  aws.String(sessionIdentifier),
		WebIdentityToken: aws.String(string(body)),
		DurationSeconds:  aws.Int64(3600),
	}

	sess, err := session.NewSession()
	if err != nil {
		log.Fatalf("error creating new AWS session: %s", err)
	}

	stsAPI := sts.New(sess)
	request, _ := stsAPI.AssumeRoleWithWebIdentityRequest(input)
	request.HTTPRequest.Header.Add(clusterIDHeader, eksClusterName)
	presignedURLString, err := request.Presign(requestPresignParam)
	if err != nil {
		log.Fatalf("error presigning AWS request: %s", err)
	}

	token := v1Prefix + base64.RawURLEncoding.EncodeToString([]byte(presignedURLString))
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
