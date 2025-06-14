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
	SearchUsers(ctx context.Context, query UserSearchQuery) ([]User, *common.Pagination, error)
}

// GORMRepository implements the Repository interface using GORM.
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository creates a new GORM user repository.
// THIS MUST RETURN THE INTERFACE TYPE: user.Repository
func NewGORMRepository(db *gorm.DB) Repository {
	return &GORMRepository{db: db}
}

// Create inserts a new user record into the database.
func (r *GORMRepository) Create(ctx context.Context, user *User) error {
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
func (r *GORMRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
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

// SearchUsers retrieves users based on search criteria and returns paginated results.
func (r *GORMRepository) SearchUsers(ctx context.Context, query UserSearchQuery) ([]User, *common.Pagination, error) {
	var users []User
	var totalCount int64

	db := r.db.WithContext(ctx).Model(&User{})

	// Apply filters
	if query.Email != nil && *query.Email != "" {
		// Case-insensitive search for email
		db = db.Where("LOWER(email) LIKE LOWER(?)", "%"+strings.TrimSpace(*query.Email)+"%")
	}
	if query.Name != nil && *query.Name != "" {
		nameQuery := "%" + strings.TrimSpace(*query.Name) + "%"
		// Case-insensitive search for name in first_name or last_name
		db = db.Where("LOWER(first_name) LIKE LOWER(?) OR LOWER(last_name) LIKE LOWER(?)", nameQuery, nameQuery)
	}
	if query.Role != nil && *query.Role != "" {
		db = db.Where("role = ?", strings.TrimSpace(*query.Role))
	}

	// Get total count before pagination
	if err := db.Count(&totalCount).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Apply pagination
	// Ensure Page and PageSize are set if they are zero (relying on UserSearchQuery embedding common.PaginationQuery which has defaults)
	page := query.Page
	if page <= 0 {
		page = common.DefaultPage
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = common.DefaultPageSize
	}

	offset := (page - 1) * pageSize
	limit := pageSize


	db = db.Offset(offset).Limit(limit)

	// TODO: Add sorting based on query.SortBy and query.SortOrder if they exist in UserSearchQuery
	// For now, default sorting by CreatedAt DESC or ID
	// db = db.Order("created_at DESC") // Example

	if err := db.Find(&users).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// It's not an error if no users match the criteria, return empty list
			return []User{}, common.NewPagination(0, page, pageSize), nil
		}
		return nil, nil, fmt.Errorf("failed to find users: %w", err)
	}

	pagination := common.NewPagination(totalCount, page, pageSize)

	return users, pagination, nil
}

// FindByFirebaseUID retrieves a user by their Firebase UID.
func (r *GORMRepository) FindByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error) {
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
func (r *GORMRepository) FindByID(ctx context.Context, id uuid.UUID) (*User, error) {
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
func (r *GORMRepository) Update(ctx context.Context, user *User) error {
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
func (r *GORMRepository) FindByProvider(ctx context.Context, authProvider string, providerID string) (*User, error) {
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
