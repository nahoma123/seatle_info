// File: internal/user/repository.go
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"seattle_info_backend/internal/common"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository defines the interface for user data operations.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	Update(ctx context.Context, user *User) error
	FindByProvider(ctx context.Context, authProvider string, providerID string) (*User, error)
	FindByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a new GORM user repository.
// THIS MUST RETURN THE INTERFACE TYPE: user.Repository
func NewGORMRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// Create inserts a new user record into the database.
func (r *gormRepository) Create(ctx context.Context, user *User) error {
	if user.Email != nil {
		*user.Email = strings.ToLower(strings.TrimSpace(*user.Email))
	}
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) ||
			strings.Contains(err.Error(), "unique constraint") ||
			strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			if user.Email != nil && strings.Contains(err.Error(), "users_email_key") {
				return common.ErrConflict.WithDetails("User with this email already exists.")
			}
			if strings.Contains(err.Error(), "unique_provider_id_per_provider") {
				return common.ErrConflict.WithDetails("This social account is already linked to a user.")
			}
			return common.ErrConflict.WithDetails("User with this email or provider ID already exists.")
		}
		return err
	}
	return nil
}

// FindByEmail retrieves a user by their email address.
func (r *gormRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var userModel User // Use a different variable name if 'user' is a parameter elsewhere causing shadow
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	err := r.db.WithContext(ctx).Where("email = ?", normalizedEmail).First(&userModel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("User not found with this email.")
		}
		return nil, err
	}
	return &userModel, nil
}

// FindByFirebaseUID retrieves a user by their Firebase UID.
func (r *gormRepository) FindByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error) {
	var userModel User
	err := r.db.WithContext(ctx).Where("firebase_uid = ?", firebaseUID).First(&userModel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("User not found with this Firebase UID.")
		}
		return nil, err
	}
	return &userModel, nil
}

// FindByID retrieves a user by their ID.
func (r *gormRepository) FindByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var userModel User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&userModel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails("User not found with this ID.")
		}
		return nil, err
	}
	return &userModel, nil
}

// Update modifies an existing user record in the database.
func (r *gormRepository) Update(ctx context.Context, user *User) error {
	if user.Email != nil {
		*user.Email = strings.ToLower(strings.TrimSpace(*user.Email))
	}
	err := r.db.WithContext(ctx).Save(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "unique constraint") {
			if user.Email != nil && strings.Contains(err.Error(), "users_email_key") {
				return common.ErrConflict.WithDetails("Update failed: email already taken.")
			}
			if strings.Contains(err.Error(), "unique_provider_id_per_provider") {
				return common.ErrConflict.WithDetails("Update failed: this social account is already linked to another user.")
			}
			return common.ErrConflict.WithDetails("Update failed due to a conflict (e.g., email already taken or social account already linked).")
		}
		return err
	}
	return nil
}

// FindByProvider retrieves a user by their OAuth provider and provider-specific ID.
func (r *gormRepository) FindByProvider(ctx context.Context, authProvider string, providerID string) (*User, error) {
	var userModel User
	err := r.db.WithContext(ctx).
		Where("auth_provider = ? AND provider_id = ?", authProvider, providerID).
		First(&userModel).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound.WithDetails(
				fmt.Sprintf("User not found with provider %s and ID %s.", authProvider, providerID),
			)
		}
		return nil, err
	}
	return &userModel, nil
}
