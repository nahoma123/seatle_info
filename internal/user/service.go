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
	// Check if user already exists by email. s.repo.FindByEmail returns a GORM user.User
	_, err := s.repo.FindByEmail(ctx, req.Email)
	if err == nil {
		// User found, so this email is already registered.
		return nil, nil, common.ErrConflict.WithDetails("User with this email already exists.")
	}
	if !errors.Is(err, common.ErrNotFound) {
		// Some other error occurred during the email check.
		return nil, nil, fmt.Errorf("failed to check existing user by email: %w", err)
	}
	// At this point, err is common.ErrNotFound, meaning email is available.

	hashedPassword, err := common.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("Failed to hash password during registration", zap.Error(err))
		return nil, nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create a GORM user.User model using the adapter
	dbUser := CreateRequestToDB(&req, hashedPassword) // user.CreateRequestToDB

	// Save the GORM user.User model to the database
	if err := s.repo.Create(ctx, dbUser); err != nil {
		s.logger.Error("Failed to create user in repository", zap.Error(err), zap.String("email", req.Email))
		// Check if the error is a known API error (e.g., conflict)
		if apiErr, ok := common.IsAPIError(err); ok {
			return nil, nil, apiErr
		}
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens using the GORM user model (dbUser implements shared.UserDataForToken)
	accessToken, accessExpiresAt, err := s.tokenService.GenerateAccessToken(dbUser)
	if err != nil {
		s.logger.Error("Failed to generate access token after registration", zap.Error(err), zap.String("userID", dbUser.ID.String()))
		// Decide if we should return an error or just log. For registration, token is crucial.
		return nil, nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, _, err := s.tokenService.GenerateRefreshToken(dbUser)
	if err != nil {
		s.logger.Error("Failed to generate refresh token after registration", zap.Error(err), zap.String("userID", dbUser.ID.String()))
		// Decide if we should return an error or just log.
		// For now, proceed without refresh token if it fails, but log it.
	}

	tokenResponse := &shared.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}

	// Convert GORM user.User to shared.User for the response
	sharedUser := DBToShared(dbUser) // user.DBToShared

	s.logger.Info("User registered successfully", zap.String("userID", sharedUser.ID.String()))
	return sharedUser, tokenResponse, nil
}

func (s *ServiceImplementation) Login(ctx context.Context, email, password string) (*shared.User, *shared.TokenResponse, error) {
	// s.repo.FindByEmail returns a GORM user.User
	dbUser, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.logger.Info("User not found during login", zap.String("email", email))
			return nil, nil, common.ErrUnauthorized.WithDetails("Invalid email or password.")
		}
		s.logger.Error("Error finding user by email during login", zap.Error(err), zap.String("email", email))
		return nil, nil, common.ErrInternalServer.WithDetails("Login failed due to an internal error.")
	}

	// dbUser is user.User which has PasswordHash
	if dbUser.PasswordHash == nil || *dbUser.PasswordHash == "" {
		s.logger.Warn("User attempting to login with email/password has no password hash (possibly OAuth user or incomplete registration)",
			zap.String("userID", dbUser.ID.String()))
		return nil, nil, common.ErrUnauthorized.WithDetails("Login with email/password not configured for this account.")
	}

	if !common.CheckPasswordHash(password, *dbUser.PasswordHash) {
		s.logger.Warn("Invalid password attempt", zap.String("userID", dbUser.ID.String()))
		return nil, nil, common.ErrUnauthorized.WithDetails("Invalid email or password.")
	}

	now := time.Now()
	dbUser.LastLoginAt = &now
	if err := s.repo.Update(ctx, dbUser); err != nil {
		// Log error but proceed with login as this is not critical for auth.
		s.logger.Error("Failed to update last login time", zap.Error(err), zap.String("userID", dbUser.ID.String()))
	}

	// Generate tokens using the GORM user model (dbUser implements shared.UserDataForToken)
	accessToken, accessExpiresAt, err := s.tokenService.GenerateAccessToken(dbUser)
	if err != nil {
		s.logger.Error("Failed to generate access token on login", zap.Error(err), zap.String("userID", dbUser.ID.String()))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not generate access token.")
	}

	refreshToken, _, err := s.tokenService.GenerateRefreshToken(dbUser)
	if err != nil {
		// Log error but proceed without refresh token if it fails.
		s.logger.Error("Failed to generate refresh token on login", zap.Error(err), zap.String("userID", dbUser.ID.String()))
		// refreshToken will be an empty string, which is acceptable.
	}

	tokenResponse := &shared.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}

	// Convert GORM user.User to shared.User for the response.
	// dbUser.Sanitize() can be called here if needed, but DBToShared handles not exposing sensitive fields.
	// The Sanitize method on user.User primarily nils out PasswordHash.
	// DBToShared already ensures PasswordHash is not part of shared.User.
	// No explicit Sanitize() call is strictly needed before DBToShared if its only purpose was PasswordHash.
	// However, if Sanitize() does other things, it could be called on dbUser.
	// For now, let's assume DBToShared is sufficient.

	sharedUser := DBToShared(dbUser) // user.DBToShared

	s.logger.Info("User logged in successfully", zap.String("userID", sharedUser.ID.String()))
	return sharedUser, tokenResponse, nil
}

func (s *ServiceImplementation) GetUserByID(ctx context.Context, id uuid.UUID) (*shared.User, error) {
	dbUser, err := s.repo.FindByID(ctx, id) // s.repo.FindByID returns GORM user.User
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.logger.Info("User not found by ID", zap.String("userID", id.String()))
		} else {
			s.logger.Error("Error finding user by ID", zap.Error(err), zap.String("userID", id.String()))
		}
		return nil, err // Return original error, which might be common.ErrNotFound or other
	}
	sharedUser := DBToShared(dbUser) // user.DBToShared
	return sharedUser, nil
}

func (s *ServiceImplementation) GetUserByEmail(ctx context.Context, email string) (*shared.User, error) {
	dbUser, err := s.repo.FindByEmail(ctx, email) // s.repo.FindByEmail returns GORM user.User
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.logger.Info("User not found by email", zap.String("email", email))
		} else {
			s.logger.Error("Error finding user by email", zap.Error(err), zap.String("email", email))
		}
		return nil, err // Return original error
	}
	sharedUser := DBToShared(dbUser) // user.DBToShared
	return sharedUser, nil
}

func (s *ServiceImplementation) FindOrCreateOrLinkOAuthUser(ctx context.Context, profile shared.OAuthUserProfile) (*shared.User, bool, error) {
	s.logger.Info("Processing OAuth user profile",
		zap.String("provider", profile.Provider),
		zap.String("providerID", profile.ProviderID),
		zap.String("email", profile.Email),
	)

	// Try to find an existing user by provider and provider ID.
	// s.repo.FindByProvider returns a GORM user.User
	dbUser, err := s.repo.FindByProvider(ctx, profile.Provider, profile.ProviderID)
	if err == nil && dbUser != nil {
		s.logger.Info("OAuth user found by provider ID", zap.String("userID", dbUser.ID.String()))

		// Store original state for comparison to see if an update is needed.
		// This simple check might not be sophisticated enough for all field types (e.g. pointers vs values if they differed)
		// but given user.UpdateRequestFromProfileToDB will handle nil checks and specific logic, this is more of a conceptual "did something change"
		originalEmail := dbUser.Email
		originalFirstName := dbUser.FirstName
		originalLastName := dbUser.LastName
		originalPictureURL := dbUser.ProfilePictureURL
		originalIsEmailVerified := dbUser.IsEmailVerified

		// Update user's profile information from OAuth provider data.
		// user.UpdateRequestFromProfileToDB handles the specific logic of what to update.
		UpdateRequestFromProfileToDB(&profile, dbUser) // This updates dbUser in place

		now := time.Now()
		dbUser.LastLoginAt = &now // Always update LastLoginAt

		// Check if any relevant field actually changed or if LastLoginAt is the only change.
		// The UpdateRequestFromProfileToDB function itself sets dbUser.UpdatedAt.
		// We need to determine if a call to s.repo.Update is truly necessary beyond LastLoginAt and UpdatedAt.
		// For simplicity, the adapter sets UpdatedAt. If we call repo.Update, it will be saved.
		// Let's assume an update is generally fine. More complex logic could compare more fields.
		profileDataChanged := (dbUser.Email != originalEmail ||
			dbUser.FirstName != originalFirstName ||
			dbUser.LastName != originalLastName ||
			dbUser.ProfilePictureURL != originalPictureURL ||
			dbUser.IsEmailVerified != originalIsEmailVerified)

		if profileDataChanged { // Only log if actual profile data changed, LastLoginAt will always update
			s.logger.Info("OAuth user profile data requires update.", zap.String("userID", dbUser.ID.String()))
		}

		if err := s.repo.Update(ctx, dbUser); err != nil {
			s.logger.Error("Failed to update existing OAuth user profile", zap.Error(err), zap.String("userID", dbUser.ID.String()))
			return nil, false, common.ErrInternalServer.WithDetails("Could not update user profile.")
		}
		s.logger.Info("OAuth user profile processed/updated", zap.String("userID", dbUser.ID.String()))
		return DBToShared(dbUser), false, nil // Convert GORM user to shared.User for return
	}
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		// Handle errors other than "not found" when looking up by provider ID.
		s.logger.Error("Error finding user by provider ID", zap.Error(err),
			zap.String("provider", profile.Provider), zap.String("providerID", profile.ProviderID))
		return nil, false, err
	}
	// At this point, err is common.ErrNotFound, meaning no user for this provider/providerID.

	// If user not found by provider ID, try to link by verified email.
	if profile.Email != "" && profile.EmailVerified {
		s.logger.Info("Attempting to link OAuth account by verified email", zap.String("email", profile.Email))
		emailLower := strings.ToLower(strings.TrimSpace(profile.Email))
		dbUserByEmail, emailErr := s.repo.FindByEmail(ctx, emailLower) // Returns GORM user.User

		if emailErr == nil && dbUserByEmail != nil {
			// User found by email. Check for provider conflicts.
			if dbUserByEmail.AuthProvider != "email" && dbUserByEmail.AuthProvider != profile.Provider {
				s.logger.Warn("User found by email but already linked to a different OAuth provider",
					zap.String("userID", dbUserByEmail.ID.String()),
					zap.String("existingProvider", dbUserByEmail.AuthProvider),
					zap.String("newProvider", profile.Provider))
				return nil, false, common.ErrConflict.WithDetails(
					fmt.Sprintf("This email is already associated with an account using %s. Cannot link %s.", dbUserByEmail.AuthProvider, profile.Provider))
			}
			// If same provider, but different ProviderID (should be rare, implies something odd)
			if dbUserByEmail.AuthProvider == profile.Provider &&
				(dbUserByEmail.ProviderID == nil || *dbUserByEmail.ProviderID != profile.ProviderID) {
				s.logger.Warn("User found by email, same provider, but different provider ID",
					zap.String("userID", dbUserByEmail.ID.String()),
					zap.Stringp("existingProviderID", dbUserByEmail.ProviderID),
					zap.String("newProviderID", profile.ProviderID))
				// This case might need specific handling depending on business rules.
				// For now, treat as a conflict.
				return nil, false, common.ErrConflict.WithDetails(
					fmt.Sprintf("This email is already linked to a %s account with a different user ID.", profile.Provider))
			}

			s.logger.Info("Linking OAuth identity to existing email user",
				zap.String("userID", dbUserByEmail.ID.String()), zap.String("provider", profile.Provider))

			// Update existing GORM user with OAuth profile information.
			UpdateRequestFromProfileToDB(&profile, dbUserByEmail) // Updates dbUserByEmail in place
			// Ensure ProviderID is explicitly set as UpdateRequestFromProfileToDB might not set it if it's for general profile updates.
			// However, our current UpdateRequestFromProfileToDB *does* set AuthProvider. Let's ensure ProviderID from the current profile is used for linking.
			dbUserByEmail.ProviderID = &profile.ProviderID // Explicitly link ProviderID

			now := time.Now()
			dbUserByEmail.LastLoginAt = &now

			if err := s.repo.Update(ctx, dbUserByEmail); err != nil {
				s.logger.Error("Failed to link OAuth account to existing user by email", zap.Error(err), zap.String("userID", dbUserByEmail.ID.String()))
				return nil, false, common.ErrInternalServer.WithDetails("Could not link OAuth account.")
			}
			s.logger.Info("OAuth identity linked to existing user", zap.String("userID", dbUserByEmail.ID.String()))
			return DBToShared(dbUserByEmail), false, nil // Convert GORM user to shared.User
		}
		if emailErr != nil && !errors.Is(emailErr, common.ErrNotFound) {
			// Handle errors other than "not found" when looking up by email.
			s.logger.Error("Error finding user by email for OAuth linking", zap.Error(emailErr), zap.String("email", profile.Email))
			return nil, false, emailErr
		}
		// At this point, (err is common.ErrNotFound for FindByProvider) AND (emailErr is common.ErrNotFound for FindByEmail)
	}

	// Create a new user if not found by provider ID or verified email.
	s.logger.Info("Creating new user from OAuth profile",
		zap.String("provider", profile.Provider), zap.String("email", profile.Email))

	// Construct a new GORM user.User model.
	// No direct adapter like CreateRequestToDB, so construct manually.
	currentTime := time.Now()
	dbNewUser := &User{ // This is user.User (GORM model)
		BaseModel: common.BaseModel{ // Explicitly initialize BaseModel for new user
			ID:        uuid.New(),
			CreatedAt: currentTime,
			UpdatedAt: currentTime,
		},
		AuthProvider:    profile.Provider,
		ProviderID:      &profile.ProviderID, // Make sure to use a copy if profile.ProviderID could change
		IsEmailVerified: profile.EmailVerified,
		Role:            "user", // Default role
		LastLoginAt:     &currentTime,
	}

	if profile.Email != "" {
		emailCopy := strings.ToLower(strings.TrimSpace(profile.Email))
		dbNewUser.Email = &emailCopy
	}
	if profile.FirstName != "" {
		firstNameCopy := profile.FirstName
		dbNewUser.FirstName = &firstNameCopy
	}
	if profile.LastName != "" {
		lastNameCopy := profile.LastName
		dbNewUser.LastName = &lastNameCopy
	}
	if profile.PictureURL != "" {
		pictureURLCopy := profile.PictureURL
		dbNewUser.ProfilePictureURL = &pictureURLCopy
	}

	if err := s.repo.Create(ctx, dbNewUser); err != nil {
		s.logger.Error("Failed to create new OAuth user in repository", zap.Error(err), zap.String("email", profile.Email))
		// Check if the error is a known API error (e.g., conflict from unique constraint on email)
		if apiErr, ok := common.IsAPIError(err); ok {
			return nil, false, apiErr
		}
		return nil, false, common.ErrInternalServer.WithDetails("Could not create new user account.")
	}

	s.logger.Info("New OAuth user created successfully", zap.String("userID", dbNewUser.ID.String()))
	return DBToShared(dbNewUser), true, nil // Convert GORM user to shared.User
}
