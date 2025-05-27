package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/shared"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ServiceImplementation implements the shared.Service interface.
type ServiceImplementation struct {
	repo         Repository        // This is user.Repository (defined in user/repository.go)
	tokenService shared.TokenService // This is shared.TokenService (defined in shared/interfaces.go)
	cfg          *config.Config    // This is config.Config (defined in config/config.go)
	logger       *zap.Logger       // This is zap.Logger (from go.uber.org/zap)
}

var _ shared.Service = (*ServiceImplementation)(nil)
var _ shared.OAuthUserProvider = (*ServiceImplementation)(nil)

// NewService creates a new user service.
func NewService(
	repo Repository, // Expects user.Repository interface
	tokenService shared.TokenService, // Expects shared.TokenService interface
	cfg *config.Config,
	logger *zap.Logger,
) *ServiceImplementation {
	return &ServiceImplementation{
		repo:         repo,
		tokenService: tokenService,
		cfg:          cfg,
		logger:       logger,
	}
}

// Register creates a new user.
func (s *ServiceImplementation) Register(ctx context.Context, req shared.CreateUserRequest) (*shared.User, *shared.TokenResponse, error) {
	existingUser, err := s.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check existing user: %w", err)
	}

	if existingUser != nil {
		return nil, nil, common.ErrUserAlreadyExists
	}

	// Hash the password
	hashedPassword, err := common.HashPassword(req.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create the user
	user := &shared.User{
		ID:        uuid.New(),
		Email:     req.Email,
		Password:  hashedPassword,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Role:      "user", // Default role for new users
	}

	// Save the user
	err = s.repo.Create(ctx, user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	tokenResponse, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return user, tokenResponse, nil
}

func (s *ServiceImplementation) Login(ctx context.Context, email, password string) (*shared.User, *shared.TokenResponse, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.logger.Info("User not found during login", zap.String("email", email))
			return nil, nil, common.ErrUnauthorized.WithDetails("Invalid email or password.")
		}
		s.logger.Error("Error finding user by email during login", zap.Error(err), zap.String("email", email))
		return nil, nil, common.ErrInternalServer.WithDetails("Login failed.")
	}

	if user.Password == "" {
		s.logger.Warn("User attempting to login with email/password has no password (possibly OAuth user)", zap.String("userID", user.ID.String()))
		return nil, nil, common.ErrUnauthorized.WithDetails("Login with email/password not configured for this account.")
	}

	if !common.CheckPasswordHash(password, user.Password) {
		return nil, nil, common.ErrUnauthorized.WithDetails("Invalid email or password.")
	}

	now := time.Now()
	user.LastLoginAt = &now
	if err := s.repo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update last login time", zap.Error(err), zap.String("userID", user.ID.String()))
	}

	accessToken, accessExpiresAt, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		s.logger.Error("Failed to generate access token on login", zap.Error(err), zap.String("userID", user.ID.String()))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not generate access token.")
	}

	refreshToken, _, err := s.tokenService.GenerateRefreshToken(user)
	if err != nil {
		s.logger.Error("Failed to generate refresh token on login", zap.Error(err), zap.String("userID", user.ID.String()))
	}

	tokenResponse := &shared.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}

	s.logger.Info("User logged in successfully", zap.String("userID", user.ID.String()))
	user.Sanitize()
	return user, tokenResponse, nil
}

func (s *ServiceImplementation) GetUserByID(ctx context.Context, id uuid.UUID) (*shared.User, error) {
	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *ServiceImplementation) GetUserByEmail(ctx context.Context, email string) (*shared.User, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *ServiceImplementation) FindOrCreateOrLinkOAuthUser(ctx context.Context, profile shared.OAuthUserProfile) (*shared.User, bool, error) {
	s.logger.Info("Processing OAuth user profile",
		zap.String("provider", profile.Provider),
		zap.String("providerID", profile.ProviderID),
		zap.String("email", profile.Email),
	)

	existingUser, err := s.repo.FindByProvider(ctx, profile.Provider, profile.ProviderID)
	if err == nil && existingUser != nil {
		s.logger.Info("OAuth user found by provider ID", zap.String("userID", existingUser.ID.String()))
		updateNeeded := false
		if profile.FirstName != "" && (existingUser.FirstName == nil || *existingUser.FirstName != profile.FirstName) {
			existingUser.FirstName = &profile.FirstName
			updateNeeded = true
		}
		if profile.LastName != "" && (existingUser.LastName == nil || *existingUser.LastName != profile.LastName) {
			existingUser.LastName = &profile.LastName
			updateNeeded = true
		}
		if profile.PictureURL != "" && (existingUser.ProfilePictureURL == nil || *existingUser.ProfilePictureURL != profile.PictureURL) {
			existingUser.ProfilePictureURL = &profile.PictureURL
			updateNeeded = true
		}

		emailLower := strings.ToLower(strings.TrimSpace(profile.Email))
		if profile.Email != "" && (existingUser.Email == nil || *existingUser.Email != emailLower) {
			if existingUser.Email != nil && *existingUser.Email != emailLower {
				otherUser, findErr := s.repo.FindByEmail(ctx, emailLower)
				if findErr == nil && otherUser != nil && otherUser.ID != existingUser.ID {
					s.logger.Error("OAuth user's new email already exists for another user during profile update",
						zap.String("userID", existingUser.ID.String()), zap.String("newEmail", emailLower))
					return nil, false, common.ErrConflict.WithDetails("Email provided by social login is already associated with another account.")
				}
			}
			existingUser.Email = &emailLower
			updateNeeded = true
		}

		if profile.EmailVerified && !existingUser.IsEmailVerified {
			existingUser.IsEmailVerified = true
			updateNeeded = true
		}

		now := time.Now()
		existingUser.LastLoginAt = &now
		updateNeeded = true

		if updateNeeded {
			if err := s.repo.Update(ctx, existingUser); err != nil {
				s.logger.Error("Failed to update existing OAuth user profile", zap.Error(err), zap.String("userID", existingUser.ID.String()))
				return nil, false, common.ErrInternalServer.WithDetails("Could not update user profile.")
			}
			s.logger.Info("OAuth user profile updated", zap.String("userID", existingUser.ID.String()))
		}
		existingUser.Sanitize()
		return existingUser, false, nil
	} else if err != nil && !errors.Is(err, common.ErrNotFound) {
		s.logger.Error("Error finding user by provider ID", zap.Error(err))
		return nil, false, err
	}

	if profile.Email != "" && profile.EmailVerified {
		s.logger.Info("Attempting to link OAuth account by email", zap.String("email", profile.Email))
		emailLower := strings.ToLower(strings.TrimSpace(profile.Email))
		userByEmail, emailErr := s.repo.FindByEmail(ctx, emailLower)
		if emailErr == nil && userByEmail != nil {
			if userByEmail.AuthProvider != "email" && userByEmail.AuthProvider != profile.Provider {
				s.logger.Error("User found by email but already linked to a different OAuth provider",
					zap.String("userID", userByEmail.ID.String()), zap.String("existingProvider", userByEmail.AuthProvider), zap.String("newProvider", profile.Provider))
				return nil, false, common.ErrConflict.WithDetails(fmt.Sprintf("This email is already associated with an account using %s. Cannot link %s.", userByEmail.AuthProvider, profile.Provider))
			}
			if userByEmail.AuthProvider == profile.Provider && userByEmail.ProviderID != nil && *userByEmail.ProviderID != profile.ProviderID {
				s.logger.Error("User found by email but already linked to this OAuth provider with a different provider ID",
					zap.String("userID", userByEmail.ID.String()), zap.Stringp("existingProviderID", userByEmail.ProviderID), zap.String("newProviderID", profile.ProviderID))
				return nil, false, common.ErrConflict.WithDetails(fmt.Sprintf("This email is already linked to a %s account with a different provider ID.", profile.Provider))
			}

			s.logger.Info("Linking OAuth identity to existing email user",
				zap.String("userID", userByEmail.ID.String()), zap.String("provider", profile.Provider))

			userByEmail.AuthProvider = profile.Provider
			providerIDCopy := profile.ProviderID
			userByEmail.ProviderID = &providerIDCopy
			if profile.FirstName != "" && (userByEmail.FirstName == nil || *userByEmail.FirstName == "") {
				userByEmail.FirstName = &profile.FirstName
			}
			if profile.LastName != "" && (userByEmail.LastName == nil || *userByEmail.LastName == "") {
				userByEmail.LastName = &profile.LastName
			}
			if profile.PictureURL != "" && (userByEmail.ProfilePictureURL == nil || *userByEmail.ProfilePictureURL == "") {
				userByEmail.ProfilePictureURL = &profile.PictureURL
			}
			if !userByEmail.IsEmailVerified {
				userByEmail.IsEmailVerified = true
			}
			now := time.Now()
			userByEmail.LastLoginAt = &now

			if err := s.repo.Update(ctx, userByEmail); err != nil {
				s.logger.Error("Failed to link OAuth account to existing user by email", zap.Error(err), zap.String("userID", userByEmail.ID.String()))
				return nil, false, common.ErrInternalServer.WithDetails("Could not link OAuth account.")
			}
			userByEmail.Sanitize()
			return userByEmail, false, nil
		} else if emailErr != nil && !errors.Is(emailErr, common.ErrNotFound) {
			s.logger.Error("Error finding user by email for OAuth linking", zap.Error(emailErr))
			return nil, false, emailErr
		}
	}

	s.logger.Info("Creating new user from OAuth profile",
		zap.String("provider", profile.Provider), zap.String("email", profile.Email))

	newUser := &User{
		AuthProvider:    profile.Provider,
		ProviderID:      &profile.ProviderID,
		IsEmailVerified: profile.EmailVerified,
		Role:            "user",
	}
	if profile.Email != "" {
		emailLower := strings.ToLower(strings.TrimSpace(profile.Email))
		newUser.Email = &emailLower
	}
	if profile.FirstName != "" {
		newUser.FirstName = &profile.FirstName
	}
	if profile.LastName != "" {
		newUser.LastName = &profile.LastName
	}
	if profile.PictureURL != "" {
		newUser.ProfilePictureURL = &profile.PictureURL
	}
	now := time.Now()
	newUser.LastLoginAt = &now

	if err := s.repo.Create(ctx, newUser); err != nil {
		s.logger.Error("Failed to create new OAuth user in repository", zap.Error(err))
		if _, ok := common.IsAPIError(err); ok {
			return nil, false, err
		}
		return nil, false, common.ErrInternalServer.WithDetails("Could not create new user account.")
	}

	s.logger.Info("New OAuth user created successfully", zap.String("userID", newUser.ID.String()))
	newUser.Sanitize()
	return newUser, true, nil
}
