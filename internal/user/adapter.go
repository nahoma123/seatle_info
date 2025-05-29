package user

import (
	"seattle_info_backend/internal/shared"
	"time" // For time.Time
)

// DBToShared converts a GORM user.User model to a shared.User DTO.
func DBToShared(dbUser *User) *shared.User {
	if dbUser == nil {
		return nil
	}
	return &shared.User{
		ID:                  dbUser.ID,
		Email:               dbUser.Email, // Assumes Email is *string in both
		FirstName:           dbUser.FirstName, // Assumes FirstName is *string in both
		LastName:            dbUser.LastName, // Assumes LastName is *string in both
		Role:                dbUser.Role,
		ProfilePictureURL:   dbUser.ProfilePictureURL,
		AuthProvider:        dbUser.AuthProvider,
		IsEmailVerified:     dbUser.IsEmailVerified,
		IsFirstPostApproved: dbUser.IsFirstPostApproved,
		CreatedAt:           dbUser.CreatedAt,
		UpdatedAt:           dbUser.UpdatedAt,
		LastLoginAt:         dbUser.LastLoginAt,
	}
}

// CreateRequestToDB was removed as shared.CreateUserRequest is obsolete.
// New user creation is now handled by GetOrCreateUserFromFirebaseClaims in user.Service.

// UpdateRequestFromProfileToDB was removed as shared.OAuthUserProfile is obsolete.
// User profile updates from Firebase are handled by GetOrCreateUserFromFirebaseClaims in user.Service.

// UpdateRequestFromSharedToDB updates an existing GORM user.User model from a shared.User DTO.
// This is for general profile updates by the user. Sensitive fields are not updated here.
func UpdateRequestFromSharedToDB(svUser *shared.User, dbUser *User) {
	if dbUser == nil || svUser == nil {
		return
	}

	// Fields a user can typically update:
	if svUser.FirstName != nil {
		dbUser.FirstName = svUser.FirstName
	}
	if svUser.LastName != nil {
		dbUser.LastName = svUser.LastName
	}
	if svUser.ProfilePictureURL != nil {
		dbUser.ProfilePictureURL = svUser.ProfilePictureURL
	}

	// Email change might require a separate verification flow, not handled by simple update.
	// Role change is an admin function, not here.
	// AuthProvider, ProviderID, IsEmailVerified are usually not directly updatable by user post-registration.

	dbUser.UpdatedAt = time.Now()
}
