package user

import (
	"seattle_info_backend/internal/common" // For common.BaseModel
	"seattle_info_backend/internal/shared"
	"time" // For time.Time

	"github.com/google/uuid" // For uuid.UUID
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

// CreateRequestToDB converts a shared.CreateUserRequest and hashed password to a GORM user.User model.
// It also sets default values for a new user.
func CreateRequestToDB(req *shared.CreateUserRequest, hashedPassword string) *User {
	currentTime := time.Now()
	emailCopy := req.Email // req.Email is string, user.User.Email is *string
	firstNameCopy := req.FirstName
	lastNameCopy := req.LastName

	return &User{
		BaseModel: common.BaseModel{
			ID:        uuid.New(),
			CreatedAt: currentTime,
			UpdatedAt: currentTime,
		},
		Email:               &emailCopy,
		PasswordHash:        &hashedPassword,
		FirstName:           &firstNameCopy,
		LastName:            &lastNameCopy,
		Role:                "user", // Default role
		AuthProvider:        "email", // Default auth provider for direct registration
		IsEmailVerified:     false, // Email not verified initially
		IsFirstPostApproved: false, // Default value
	}
}

// UpdateRequestFromProfileToDB updates an existing GORM user.User model from a shared.OAuthUserProfile.
// It focuses on updating fields typically provided by OAuth.
func UpdateRequestFromProfileToDB(profile *shared.OAuthUserProfile, dbUser *User) {
	if dbUser == nil || profile == nil {
		return
	}

	dbUser.AuthProvider = profile.Provider
	// ProviderID is usually set when linking, not directly here unless it's a new user creation path.
	// This function assumes dbUser already exists and is being updated post-OAuth identification.

	if profile.Email != "" {
		// Only update email if provided by OAuth and potentially if verified
		// More complex logic might be needed if email changes require re-verification
		emailCopy := profile.Email
		dbUser.Email = &emailCopy
	}
	if profile.FirstName != "" {
		firstNameCopy := profile.FirstName
		dbUser.FirstName = &firstNameCopy
	}
	if profile.LastName != "" {
		lastNameCopy := profile.LastName
		dbUser.LastName = &lastNameCopy
	}
	if profile.PictureURL != "" {
		pictureURLCopy := profile.PictureURL
		dbUser.ProfilePictureURL = &pictureURLCopy
	}
	if profile.EmailVerified {
		dbUser.IsEmailVerified = true
	}
	// LastLoginAt should be updated by the service layer after successful login/linking
	dbUser.UpdatedAt = time.Now()
}

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
