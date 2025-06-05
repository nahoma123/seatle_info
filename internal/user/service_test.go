package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"

	// Mocking library like testify/mock can be added later:
	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/mock"

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MockUserRepository is a mock implementation of the user.Repository interface.
// (Define this mock or use a library like testify/mock later)
type MockUserRepository struct {
	// Mock fields and methods would go here.
	// For example:
	// FindByFirebaseUIDFunc func(ctx context.Context, firebaseUID string) (*User, error)
	// CreateFunc func(ctx context.Context, user *User) error
	// UpdateFunc func(ctx context.Context, user *User) error
	// FindByEmailFunc func(ctx context.Context, email string) (*User, error)
	// FindByIDFunc func(ctx context.Context, id uuid.UUID) (*User, error)
	// FindByProviderFunc func(ctx context.Context, provider, providerID string) (*User, error)
}

// Implement Repository interface for MockUserRepository (actual mocking logic to be filled in)
func (m *MockUserRepository) FindByFirebaseUID(ctx context.Context, firebaseUID string) (*User, error) {
	// Example: return m.FindByFirebaseUIDFunc(ctx, firebaseUID)
	if firebaseUID == "existing_fb_uid" {
		email := "test@example.com"
		name := "Test User"
		now := time.Now()
		return &User{
			BaseModel:   common.BaseModel{ID: uuid.New(), CreatedAt: now, UpdatedAt: now},
			FirebaseUID: &firebaseUID,
			Email:       &email,
			FirstName:   &name,
			Role:        common.RoleUser,
		}, nil
	}
	if firebaseUID == "error_case_uid" {
		return nil, errors.New("mock repository error")
	}
	return nil, common.ErrNotFound // Default to not found
}

func (m *MockUserRepository) Create(ctx context.Context, user *User) error {
	// Example: return m.CreateFunc(ctx, user)
	return nil // Assume success for now
}

func (m *MockUserRepository) Update(ctx context.Context, user *User) error {
	// Example: return m.UpdateFunc(ctx, user)
	return nil // Assume success for now
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return nil, common.ErrNotFound
}
func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return nil, common.ErrNotFound
}
func (m *MockUserRepository) FindByProvider(ctx context.Context, provider, providerID string) (*User, error) {
	return nil, common.ErrNotFound
}

func TestUserService_GetOrCreateUserFromFirebaseClaims(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{} // Basic config, add fields if service needs them

	mockRepo := &MockUserRepository{}
	userService := NewService(mockRepo, cfg, logger) // Pass mockRepo

	// Sample Firebase token for testing
	// In real tests, you might need more elaborate ways to create/mock firebaseauth.Token
	sampleFirebaseToken := &firebaseauth.Token{
		UID: "new_fb_uid",
		Claims: map[string]interface{}{
			"email":          "newuser@example.com",
			"email_verified": true,
			"name":           "New User",
			"picture":        "http://example.com/new_pic.jpg",
		},
		Firebase: firebaseauth.FirebaseInfo{SignInProvider: "google.com"},
	}

	sampleExistingFirebaseToken := &firebaseauth.Token{
		UID: "existing_fb_uid", // Matches UID in mockRepo.FindByFirebaseUID
		Claims: map[string]interface{}{
			"email":          "test@example.com", // Same email as in mock
			"email_verified": true,
			"name":           "Test User Updated Name", // Updated name
			"picture":        "http://example.com/existing_pic_updated.jpg",
		},
		Firebase: firebaseauth.FirebaseInfo{SignInProvider: "google.com"},
	}

	ctx := context.Background()

	tests := []struct {
		name               string
		firebaseToken      *firebaseauth.Token
		setupMock          func(token *firebaseauth.Token) // To setup specific mock behavior per test
		wantErr            bool
		wantWasCreated     bool
		expectedEmail      *string
		expectedName       *string
		expectedProviderID *string
	}{
		{
			name:          "New Firebase user - should create local user",
			firebaseToken: sampleFirebaseToken,
			setupMock: func(token *firebaseauth.Token) {
				// mockRepo.FindByFirebaseUIDFunc = func ... (return common.ErrNotFound)
				// mockRepo.CreateFunc = func ... (return nil)
			},
			wantErr:            false,
			wantWasCreated:     true,
			expectedEmail:      func() *string { e := "newuser@example.com"; return &e }(),
			expectedName:       func() *string { n := "New User"; return &n }(),
			expectedProviderID: func() *string { p := "google.com"; return &p }(),
		},
		{
			name:          "Existing Firebase user - should find and update local user",
			firebaseToken: sampleExistingFirebaseToken,
			setupMock: func(token *firebaseauth.Token) {
				// mockRepo.FindByFirebaseUIDFunc = func ... (return existing user)
				// mockRepo.UpdateFunc = func ... (return nil)
			},
			wantErr:        false,
			wantWasCreated: false,
			expectedEmail:  func() *string { e := "test@example.com"; return &e }(),       // Assuming email doesn't change or is verified
			expectedName:   func() *string { n := "Test User Updated Name"; return &n }(), // Name should be updated
		},
		// Add more test cases:
		// - User exists, no profile data change
		// - Error from FindByFirebaseUID (other than NotFound)
		// - Error from Create
		// - Error from Update
		// - Firebase token with missing optional claims (name, picture)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMock != nil {
				tt.setupMock(tt.firebaseToken)
			}

			sharedUser, wasCreated, err := userService.GetOrCreateUserFromFirebaseClaims(ctx, tt.firebaseToken)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetOrCreateUserFromFirebaseClaims() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if wasCreated != tt.wantWasCreated {
				t.Errorf("GetOrCreateUserFromFirebaseClaims() wasCreated = %v, wantWasCreated %v", wasCreated, tt.wantWasCreated)
			}
			if !tt.wantErr && sharedUser != nil {
				if tt.expectedEmail != nil && (sharedUser.Email == nil || *sharedUser.Email != *tt.expectedEmail) {
					t.Errorf("GetOrCreateUserFromFirebaseClaims() email = %v, want %v", sharedUser.Email, tt.expectedEmail)
				}
				// This check is simplified; actual user.User model has FirstName, not a single Name.
				// The service logic splits firebaseToken.Claims["name"] into FirstName.
				// This test should reflect that by checking sharedUser.FirstName.
				if tt.expectedName != nil && (sharedUser.FirstName == nil || *sharedUser.FirstName != *tt.expectedName) {
					t.Errorf("GetOrCreateUserFromFirebaseClaims() name (FirstName) = %v, want %v", sharedUser.FirstName, tt.expectedName)
				}
				// Add checks for provider ID if it's set on shared.User or underlying user.User
			}
			// Add more assertions using testify/assert later for cleaner checks
			// e.g., assert.NoError(t, err)
			// assert.Equal(t, tt.wantWasCreated, wasCreated)
		})
	}
}

func TestUserService_GetUserByFirebaseUID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{}
	mockRepo := &MockUserRepository{}
	userService := NewService(mockRepo, cfg, logger)

	ctx := context.Background()

	tests := []struct {
		name          string
		firebaseUID   string
		setupMock     func(uid string) // To setup specific mock behavior per test
		wantErr       bool
		expectedEmail *string
	}{
		{
			name:          "User found by Firebase UID",
			firebaseUID:   "existing_fb_uid", // Matches UID in mockRepo.FindByFirebaseUID
			setupMock:     func(uid string) { /* mockRepo.FindByFirebaseUIDFunc = ... */ },
			wantErr:       false,
			expectedEmail: func() *string { e := "test@example.com"; return &e }(),
		},
		{
			name:        "User not found by Firebase UID",
			firebaseUID: "non_existing_fb_uid",
			setupMock:   func(uid string) { /* mockRepo.FindByFirebaseUIDFunc = ... return common.ErrNotFound */ },
			wantErr:     true, // Expect common.ErrNotFound
		},
		{
			name:        "Repository error",
			firebaseUID: "error_case_uid", // Matches UID in mockRepo for error
			setupMock:   func(uid string) { /* mockRepo.FindByFirebaseUIDFunc = ... return errors.New("...") */ },
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMock != nil {
				tt.setupMock(tt.firebaseUID)
			}

			sharedUser, err := userService.GetUserByFirebaseUID(ctx, tt.firebaseUID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUserByFirebaseUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && sharedUser != nil {
				if tt.expectedEmail != nil && (sharedUser.Email == nil || *sharedUser.Email != *tt.expectedEmail) {
					t.Errorf("GetUserByFirebaseUID() email = %v, want %v", sharedUser.Email, tt.expectedEmail)
				}
				// Add more specific assertions for user fields.
			}
			if tt.wantErr && err != nil && errors.Is(err, common.ErrNotFound) && tt.firebaseUID == "non_existing_fb_uid" {
				// Correctly expecting ErrNotFound
			} else if tt.wantErr && err != nil && tt.firebaseUID == "error_case_uid" && err.Error() == "mock repository error" {
				// Correctly expecting specific mock error
			} else if tt.wantErr && err == nil {
				t.Errorf("GetUserByFirebaseUID() expected error but got nil")
			}

		})
	}
}

// Note: This is a basic outline. Full implementation would require:
// 1. A proper mocking library (like testify/mock) for MockUserRepository to set expectations
//    and return values for different test cases dynamically.
// 2. More detailed assertions for all relevant fields in shared.User.
// 3. Testing of edge cases (e.g., nil claims in firebaseToken, error propagation from repository).
// 4. Configuration of mockRepo per test case within the tt.setupMock functions.
// 5. The `user.ServiceImplementation` logic for splitting `firebaseToken.Claims["name"]` into
//    `FirstName` (and potentially `LastName`) needs to be accurately reflected in test expectations.
