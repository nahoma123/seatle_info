package user

import (
	"context"
	"errors"
	"strings"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/shared"
)

// ServiceImplementation implements the shared.Service interface.
type ServiceImplementation struct {
	repo   Repository     // This is user.Repository (defined in user/repository.go)
	cfg    *config.Config // This is config.Config (defined in config/config.go)
	logger *zap.Logger    // This is zap.Logger (from go.uber.org/zap)
}

var _ shared.Service = (*ServiceImplementation)(nil)

// NewService creates a new user service.
func NewService(
	repo Repository, // Expects user.Repository interface
	cfg *config.Config,
	logger *zap.Logger,
) *ServiceImplementation {
	return &ServiceImplementation{
		repo:   repo,
		cfg:    cfg,
		logger: logger,
	}
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

// GetOrCreateUserFromFirebaseClaims handles user lookup or creation based on Firebase token claims.
func (s *ServiceImplementation) GetOrCreateUserFromFirebaseClaims(ctx context.Context, firebaseToken *firebaseauth.Token) (*shared.User, bool, error) {
	s.logger.Info("Processing Firebase user claims", zap.String("firebaseUID", firebaseToken.UID))

	dbUser, err := s.repo.FindByFirebaseUID(ctx, firebaseToken.UID)
	wasCreated := false

	if err == nil { // User found
		s.logger.Debug("User found by Firebase UID", zap.String("firebaseUID", firebaseToken.UID), zap.String("localUserID", dbUser.ID.String()))
		needsUpdate := false

		// Check and update email if necessary
		if emailClaim, ok := firebaseToken.Claims["email"].(string); ok && emailClaim != "" {
			emailVerifiedClaim, _ := firebaseToken.Claims["email_verified"].(bool) // Default to false if not present
			normalizedEmailClaim := strings.ToLower(strings.TrimSpace(emailClaim))

			if dbUser.Email == nil || *dbUser.Email != normalizedEmailClaim || dbUser.IsEmailVerified != emailVerifiedClaim {
				dbUser.Email = &normalizedEmailClaim
				dbUser.IsEmailVerified = emailVerifiedClaim
				needsUpdate = true
				s.logger.Info("Updating user email/verified status from Firebase token",
					zap.String("firebaseUID", firebaseToken.UID),
					zap.String("newEmail", normalizedEmailClaim),
					zap.Bool("newEmailVerified", emailVerifiedClaim))
			}
		}

		// Check and update name if necessary
		// Firebase 'name' claim can be split. For simplicity, using it for FirstName.
		if nameClaim, ok := firebaseToken.Claims["name"].(string); ok && nameClaim != "" {
			// Basic split for First/Last name. More robust parsing may be needed.
			// For now, if name exists and FirstName is empty or different, update FirstName.
			// This example prioritizes the 'name' claim for FirstName.
			// A more complex logic could be: if FirstName is empty, set it. If different, decide policy.
			if dbUser.FirstName == nil || *dbUser.FirstName != nameClaim { // Simplified: using full name for FirstName
				dbUser.FirstName = &nameClaim
				// LastName could be cleared or handled separately if full name is now in FirstName
				// dbUser.LastName = nil // Example: if you decide to overwrite
				needsUpdate = true
				s.logger.Info("Updating user FirstName from Firebase token 'name' claim",
					zap.String("firebaseUID", firebaseToken.UID),
					zap.String("newName", nameClaim))
			}
		}
		
		// Check and update profile picture URL
		if pictureClaim, ok := firebaseToken.Claims["picture"].(string); ok && pictureClaim != "" {
		    if dbUser.ProfilePictureURL == nil || *dbUser.ProfilePictureURL != pictureClaim {
		        dbUser.ProfilePictureURL = &pictureClaim
		        needsUpdate = true
		        s.logger.Info("Updating user ProfilePictureURL from Firebase token",
					zap.String("firebaseUID", firebaseToken.UID))
		    }
		}


		if needsUpdate {
			dbUser.UpdatedAt = time.Now()
			if err := s.repo.Update(ctx, dbUser); err != nil {
				s.logger.Error("Failed to update existing user from Firebase claims", zap.Error(err), zap.String("firebaseUID", firebaseToken.UID))
				return nil, false, common.ErrInternalServer.WithDetails("Could not update user profile from Firebase.")
			}
			s.logger.Info("User profile updated from Firebase claims", zap.String("firebaseUID", firebaseToken.UID), zap.String("localUserID", dbUser.ID.String()))
		}
		now := time.Now()
		dbUser.LastLoginAt = &now // Always update LastLoginAt, even if no other profile changes
		if err := s.repo.Update(ctx, dbUser); err != nil && needsUpdate == false { // Only update if not already updated above
			s.logger.Warn("Failed to update LastLoginAt for user", zap.Error(err), zap.String("firebaseUID", firebaseToken.UID))
			// Non-critical, proceed
		}


	} else if errors.Is(err, common.ErrNotFound) { // User not found, create new
		s.logger.Info("User not found by Firebase UID, creating new user.", zap.String("firebaseUID", firebaseToken.UID))
		wasCreated = true
		currentTime := time.Now()

		dbNewUser := &User{ // This is user.User (GORM model)
			BaseModel: common.BaseModel{
				ID:        uuid.New(),
				CreatedAt: currentTime,
				UpdatedAt: currentTime,
			},
			FirebaseUID:  &firebaseToken.UID,
			AuthProvider: "firebase", // Generic provider for Firebase
			Role:         common.RoleUser,    // Default role
			LastLoginAt:  &currentTime,
		}

		if emailClaim, ok := firebaseToken.Claims["email"].(string); ok && emailClaim != "" {
			normalizedEmail := strings.ToLower(strings.TrimSpace(emailClaim))
			dbNewUser.Email = &normalizedEmail
		}
		if emailVerifiedClaim, ok := firebaseToken.Claims["email_verified"].(bool); ok {
			dbNewUser.IsEmailVerified = emailVerifiedClaim
		}
		if nameClaim, ok := firebaseToken.Claims["name"].(string); ok && nameClaim != "" {
			// Splitting name into FirstName and LastName can be complex.
			// Simple approach: use the full name for FirstName if available.
			// More sophisticated: split by space, handle multiple spaces, titles, etc.
			// For now, assign to FirstName, and LastName can be empty or derived.
			dbNewUser.FirstName = &nameClaim
			// Example: parts := strings.Fields(nameClaim); if len(parts) > 0 { dbNewUser.FirstName = &parts[0]; if len(parts) > 1 { lastName := strings.Join(parts[1:], " "); dbNewUser.LastName = &lastName } }
		}
		if pictureClaim, ok := firebaseToken.Claims["picture"].(string); ok && pictureClaim != "" {
			dbNewUser.ProfilePictureURL = &pictureClaim
		}

		// Store the original Firebase sign-in provider (e.g., "google.com", "password")
		if firebaseInfo, ok := firebaseToken.Claims["firebase"].(map[string]interface{}); ok {
			// Now signInProvider is not needed, so we can use the blank identifier _
			if _, ok := firebaseInfo["sign_in_provider"].(string); ok {
				// dbNewUser.ProviderID = &signInProvider // Intentionally commented out as per requirements
			}
		}


		if errCreate := s.repo.Create(ctx, dbNewUser); errCreate != nil {
			s.logger.Error("Failed to create new user from Firebase claims", zap.Error(errCreate), zap.String("firebaseUID", firebaseToken.UID))
			// Handle potential conflict if by some race condition or data inconsistency, email already exists
			if apiErr, ok := common.IsAPIError(errCreate); ok && errors.Is(apiErr, common.ErrConflict) {
				return nil, false, apiErr
			}
			return nil, false, common.ErrInternalServer.WithDetails("Could not create new user account from Firebase.")
		}
		s.logger.Info("New user created successfully from Firebase claims", zap.String("firebaseUID", firebaseToken.UID), zap.String("localUserID", dbNewUser.ID.String()))
		dbUser = dbNewUser // Assign to dbUser to be returned
	} else { // Other error
		s.logger.Error("Error finding user by Firebase UID", zap.Error(err), zap.String("firebaseUID", firebaseToken.UID))
		return nil, false, common.ErrInternalServer.WithDetails("Failed to retrieve user by Firebase UID.")
	}

	return DBToShared(dbUser), wasCreated, nil
}

// GetUserByFirebaseUID retrieves a user by their Firebase UID.
func (s *ServiceImplementation) GetUserByFirebaseUID(ctx context.Context, firebaseUID string) (*shared.User, error) {
	dbUser, err := s.repo.FindByFirebaseUID(ctx, firebaseUID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			s.logger.Info("User not found by Firebase UID", zap.String("firebaseUID", firebaseUID))
		} else {
			s.logger.Error("Error finding user by Firebase UID", zap.Error(err), zap.String("firebaseUID", firebaseUID))
		}
		return nil, err // Return original error
	}
	return DBToShared(dbUser), nil
}
