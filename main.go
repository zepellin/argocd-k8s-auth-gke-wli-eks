package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"argocd-k8s-auth-gke-wli-eks/pkg/aws"
	"argocd-k8s-auth-gke-wli-eks/pkg/cache"
	"argocd-k8s-auth-gke-wli-eks/pkg/config"
	"argocd-k8s-auth-gke-wli-eks/pkg/gcp"
	"argocd-k8s-auth-gke-wli-eks/pkg/k8s"
	"argocd-k8s-auth-gke-wli-eks/pkg/logger"
)

const (
	presignedURLExpiration = 30 * time.Minute
)

// gcpTokenRetriever implements aws.TokenRetriever interface
type gcpTokenRetriever struct {
	token []byte
}

func (t *gcpTokenRetriever) GetIdentityToken() ([]byte, error) {
	return t.token, nil
}

func run(ctx context.Context) error {
	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.LoadFromFlags(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger with configured level
	if err := logger.Initialize(logger.Config{
		Level:     0, // Base level
		Verbosity: cfg.LogVerbosity,
		ToFile:    cfg.LogToFile,
	}); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Flush()

	// Initialize cache if enabled
	var credCache *cache.Cache
	if cfg.Cache {
		logger.Debug("initializing credential cache")
		var err error
		credCache, err = cache.NewCache()
		if err != nil {
			return fmt.Errorf("failed to initialize cache: %w", err)
		}
	}

	// Create cache key
	cacheKey := cache.CacheKey{
		AWSRoleARN:     cfg.AWSRoleARN,
		EKSClusterName: cfg.EKSClusterName,
		STSRegion:      cfg.STSRegion,
	}

	// Check cache for existing credentials
	if cfg.Cache && credCache != nil {
		if cachedCred, found := credCache.Get(cacheKey); found {
			logger.Debug("using cached credentials")
			if _, err := fmt.Fprint(os.Stdout, string(cachedCred)); err != nil {
				return fmt.Errorf("failed to write cached credential: %w", err)
			}
			return nil
		}
	}

	// Initialize metadata provider based on configuration
	var metadataProvider gcp.MetadataProvider
	if cfg.HybridMode {
		logger.Debug("running in hybrid mode")
		metadataProvider = gcp.NewHybridMetadataProvider(cfg.HTTPTimeout)
	} else {
		logger.Debug("running in GCP-only mode")
		metadataProvider = gcp.NewMetadataProvider(cfg.HTTPTimeout)
	}

	// Get session identifier
	sessionID, err := metadataProvider.CreateSessionIdentifier(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session identifier: %w", err)
	}

	logger.Debug("created session identifier: sessionID=%s", sessionID)

	// Get GCP identity token
	gcpToken, err := metadataProvider.GetIdentityToken(ctx, "gcp")
	if err != nil {
		return fmt.Errorf("failed to get GCP identity token: %w", err)
	}

	// Create token retriever for AWS authentication
	tokenRetriever := &gcpTokenRetriever{token: gcpToken}

	// Initialize AWS authenticator
	awsAuth, err := aws.NewAuthenticator(ctx, cfg.AWSRoleARN, sessionID, cfg.STSRegion, tokenRetriever)
	if err != nil {
		return fmt.Errorf("failed to create AWS authenticator: %w", err)
	}

	// Get AWS credentials
	awsCreds, err := awsAuth.GetCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}

	logger.Debug("retrieved AWS credentials")

	// Get presigned URL
	presignedURL, err := awsAuth.GetPresignedCallerIdentityURL(ctx, cfg.EKSClusterName, awsCreds)
	if err != nil {
		return fmt.Errorf("failed to get presigned URL: %w", err)
	}

	// Generate Kubernetes ExecCredential
	credGen := k8s.NewCredentialGenerator()
	execCred, err := credGen.GenerateExecCredential(
		presignedURL,
		time.Now().Add(presignedURLExpiration),
	)
	if err != nil {
		return fmt.Errorf("failed to generate exec credential: %w", err)
	}

	// Cache the credential if caching is enabled
	if cfg.Cache && credCache != nil {
		if err := credCache.Put(cacheKey, execCred, time.Now().Add(presignedURLExpiration)); err != nil {
			logger.Debug("failed to cache credential: %v", err)
		}
	}

	// Write the credential to stdout
	if _, err := fmt.Fprint(os.Stdout, string(execCred)); err != nil {
		return fmt.Errorf("failed to write exec credential: %w", err)
	}

	return nil
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		// Initialize minimal logger for fatal errors
		if err := logger.Initialize(logger.Config{
			Level:     0,
			Verbosity: 0,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}
		logger.Errorf(err, "program failed")
		os.Exit(1)
	}
}
