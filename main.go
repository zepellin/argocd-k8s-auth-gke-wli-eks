package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"argocd-k8s-auth-gke-wli-eks/pkg/aws"
	"argocd-k8s-auth-gke-wli-eks/pkg/config"
	"argocd-k8s-auth-gke-wli-eks/pkg/gcp"
	"argocd-k8s-auth-gke-wli-eks/pkg/k8s"
)

const (
	presignedURLExpiration = 15 * time.Minute
)

// gcpTokenRetriever implements aws.TokenRetriever interface
type gcpTokenRetriever struct {
	token []byte
}

func (t *gcpTokenRetriever) GetIdentityToken() ([]byte, error) {
	return t.token, nil
}

// getLogLevel converts string level to slog.Level
func getLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func run(ctx context.Context) error {
	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.LoadFromFlags(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger with configured level
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     getLogLevel(cfg.LogLevel),
		AddSource: true,
	}))

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

	logger.Debug("created session identifier", "sessionID", sessionID)

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

	// Write the credential to stdout
	if _, err := fmt.Fprint(os.Stdout, string(execCred)); err != nil {
		return fmt.Errorf("failed to write exec credential: %w", err)
	}

	return nil
}

func main() {
	ctx := context.Background()

	// Create default logger for main function
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	}))

	if err := run(ctx); err != nil {
		logger.Error("program failed", "error", err)
		os.Exit(1)
	}
}
