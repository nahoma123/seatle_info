package firebase

import (
	"context"
	"fmt"
	"path/filepath" // For cleaning the path

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"go.uber.org/zap"
	"google.golang.org/api/option"

	"seattle_info_backend/internal/config" // Assuming this path is correct
)

// FirebaseService provides methods to interact with Firebase services, primarily authentication.
type FirebaseService struct {
	authClient *auth.Client
	logger     *zap.Logger
}

// NewFirebaseService initializes the Firebase Admin SDK and creates a new FirebaseService.
// It takes the application config and a logger as input.
func NewFirebaseService(cfg *config.Config, logger *zap.Logger) (*FirebaseService, error) {
	if cfg.FirebaseServiceAccountKeyPath == "" {
		logger.Error("Firebase service account key path is not configured.")
		return nil, fmt.Errorf("firebase service account key path is required")
	}

	// Clean the path to prevent issues with relative paths or symlinks
	// Note: Consider security implications if path is user-supplied. Here it's from config.
	cleanPath := filepath.Clean(cfg.FirebaseServiceAccountKeyPath)

	opt := option.WithCredentialsFile(cleanPath)
	
	var app *firebase.App
	var err error

	if cfg.FirebaseProjectID != "" {
		conf := &firebase.Config{ProjectID: cfg.FirebaseProjectID}
		app, err = firebase.NewApp(context.Background(), conf, opt)
	} else {
		// If ProjectID is not specified in config, let SDK infer from credentials
		app, err = firebase.NewApp(context.Background(), nil, opt)
	}

	if err != nil {
		logger.Error("Failed to initialize Firebase Admin SDK app", zap.Error(err), zap.String("keyPath", cleanPath))
		return nil, fmt.Errorf("error initializing Firebase app: %w", err)
	}

	authClient, err := app.Auth(context.Background())
	if err != nil {
		logger.Error("Failed to get Firebase Auth client", zap.Error(err))
		return nil, fmt.Errorf("error getting Firebase Auth client: %w", err)
	}

	logger.Info("Firebase Admin SDK initialized successfully.")
	return &FirebaseService{
		authClient: authClient,
		logger:     logger,
	}, nil
}

// VerifyIDToken verifies a Firebase ID token and returns the token claims.
// It takes a context and the ID token string as input.
func (s *FirebaseService) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	if idToken == "" {
		return nil, fmt.Errorf("ID token must not be empty")
	}

	token, err := s.authClient.VerifyIDToken(ctx, idToken)
	if err != nil {
		s.logger.Warn("Firebase ID token verification failed", zap.Error(err))
		// Consider mapping firebase auth errors to common.APIError types if needed
		return nil, fmt.Errorf("failed to verify Firebase ID token: %w", err)
	}

	s.logger.Debug("Firebase ID token verified successfully", zap.String("uid", token.UID))
	return token, nil
}

// RevokeRefreshTokens revokes all refresh tokens for a given user.
func (s *FirebaseService) RevokeRefreshTokens(ctx context.Context, uid string) error {
	if err := s.authClient.RevokeRefreshTokens(ctx, uid); err != nil {
		s.logger.Error("Failed to revoke refresh tokens", zap.Error(err), zap.String("uid", uid))
		return fmt.Errorf("failed to revoke refresh tokens: %w", err)
	}
	s.logger.Info("Successfully revoked refresh tokens for user", zap.String("uid", uid))
	return nil
}