package integration_test

import (
	"context"
	"os"
	"testing"
	// "time" // Unused import

	firebaseauth "firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	// "go.uber.org/zap" // Not directly used in code logic

	// "seattle_info_backend/internal/common" // Not directly used in code logic
	"seattle_info_backend/internal/config"
	// "seattle_info_backend/internal/platform/database" // Not directly used as db.AutoMigrate is used
	"seattle_info_backend/internal/platform/logger"
	"seattle_info_backend/internal/user"
)

// Helper function to set up the test environment
func setupUserServiceTest(t *testing.T) (*user.ServiceImplementation, user.Repository, *gorm.DB) { // Changed GORMRepository to Repository interface
	t.Helper()

	// 0. Setup Configuration (minimal for this test)
	// Based on internal/config/config.go, the Config struct is flat.
	// We use an in-memory SQLite database for this test.
	cfg := &config.Config{
		DBSource:  "file::memory:?cache=shared", // For GORM with SQLite
		GinMode:   "debug",                      // Affects logger behavior
		LogLevel:  "debug",                      // For logger
		LogFormat: "console",                    // For logger
		// Firebase related fields might be needed if service initialization touches them,
		// but for this specific test, GetOrCreateUserFromFirebaseClaims mocks Firebase data.
		// Ensure required fields for service/repo init are present if any.
		// From config.Load(), FIREBASE_SERVICE_ACCOUNT_KEY_PATH is checked.
		// We should provide a dummy value or ensure the codepath isn't hit in this test setup.
		// For now, hoping service/repo init doesn't strictly need a valid path for this test.
		// If it does, we'll need to create a dummy file or mock further.
		// Let's assume user.NewService doesn't immediately try to load Firebase credentials.
		FirebaseServiceAccountKeyPath: "dummy-firebase-key.json", // Provide a dummy path
	}
	// Create a dummy key file to satisfy config loading, if it were to run full Load()
	// For this test setup, we are manually creating config, so direct file check might not run
	// unless NewService or NewGORMRepository internally calls config.Load() or similar.
	// It's safer to assume it might.
	if _, errFsStat := os.Stat(cfg.FirebaseServiceAccountKeyPath); os.IsNotExist(errFsStat) {
		_ = os.WriteFile(cfg.FirebaseServiceAccountKeyPath, []byte("{}"), 0644)
		// defer os.Remove(cfg.FirebaseServiceAccountKeyPath) // Clean up dummy file
	}


	// 1. Initialize Logger
	// Based on internal/platform/logger/zap.go, New expects *config.Config
	appLogger, err := logger.New(cfg)
	require.NoError(t, err, "Failed to initialize logger")

	// 2. Connect to Test Database (SQLite in-memory)
	// Ensure we are using gorm.io/driver/sqlite
	// DSN is now directly from cfg.DBSource
	db, err := gorm.Open(sqlite.Open(cfg.DBSource), &gorm.Config{})
	require.NoError(t, err, "Failed to connect to test database")

	// Clean up any existing tables (optional, but good for repeatable tests)
	// Order matters if there are foreign keys.
	// For this specific test, we only care about the users table.
	err = db.Migrator().DropTable(&user.User{})
	require.NoError(t, err, "Failed to drop user table")


	// 3. Run Migrations
	// We need the user.User struct to be registered with GORM for AutoMigrate to work.
	// The user.User struct is defined in internal/user/user.go (GORM model)
	err = db.AutoMigrate(&user.User{})
	require.NoError(t, err, "Failed to migrate database")

	// Verify that the table exists (optional sanity check)
	hasTable := db.Migrator().HasTable(&user.User{})
	require.True(t, hasTable, "users table should exist after migration")


	// 4. Create Repository and Service
	// Based on internal/user/repository.go, NewGORMRepository expects only *gorm.DB
	userRepo := user.NewGORMRepository(db) // Removed appLogger
	userService := user.NewService(userRepo, cfg, appLogger)

	return userService, userRepo, db
}

func TestGetOrCreateUserFromFirebaseClaims_MultipleFirebaseUsersSameSignInProvider(t *testing.T) {
	userService, userRepo, db := setupUserServiceTest(t)
	ctx := context.Background()

	// Clean up database and dummy firebase key file after test
	sqlDB, errDb := db.DB()
	require.NoError(t, errDb)
	defer sqlDB.Close()
	// Assuming FirebaseServiceAccountKeyPath was set in cfg and dummy file created
	cfgForCleanup := &config.Config{FirebaseServiceAccountKeyPath: "dummy-firebase-key.json"}
	defer os.Remove(cfgForCleanup.FirebaseServiceAccountKeyPath) // Clean up dummy file

	// Define two distinct Firebase tokens with the same sign_in_provider
	token1 := &firebaseauth.Token{
		UID: "testFirebaseUID1",
		Claims: map[string]interface{}{
			"email":          "user1@test.com",
			"name":           "Test User One",
			"email_verified": true,
			"firebase": map[string]interface{}{
				"sign_in_provider": "google.com",
			},
		},
	}

	token2 := &firebaseauth.Token{
		UID: "testFirebaseUID2",
		Claims: map[string]interface{}{
			"email":          "user2@test.com",
			"name":           "Test User Two",
			"email_verified": true,
			"firebase": map[string]interface{}{
				"sign_in_provider": "google.com", // Same sign_in_provider
			},
		},
	}

	// --- First User ---
	sharedUser1, wasCreated1, err1 := userService.GetOrCreateUserFromFirebaseClaims(ctx, token1)
	require.NoError(t, err1, "GetOrCreateUserFromFirebaseClaims failed for user 1")
	require.True(t, wasCreated1, "User 1 should have been created")
	require.NotNil(t, sharedUser1, "Shared user 1 should not be nil")

	// Retrieve user1 from DB and verify ProviderID and AuthProvider
	dbUser1, errDb1 := userRepo.FindByFirebaseUID(ctx, token1.UID)
	require.NoError(t, errDb1, "Failed to find user 1 by Firebase UID")
	require.NotNil(t, dbUser1, "DB User 1 should not be nil")
	assert.Nil(t, dbUser1.ProviderID, "User 1 ProviderID should be nil")
	assert.Equal(t, "firebase", dbUser1.AuthProvider, "User 1 AuthProvider should be 'firebase'")
	assert.Equal(t, token1.UID, *dbUser1.FirebaseUID, "User 1 FirebaseUID should match")
	expectedEmail1 := "user1@test.com"
	assert.Equal(t, expectedEmail1, *dbUser1.Email, "User 1 Email should match")

	// --- Second User ---
	sharedUser2, wasCreated2, err2 := userService.GetOrCreateUserFromFirebaseClaims(ctx, token2)
	require.NoError(t, err2, "GetOrCreateUserFromFirebaseClaims failed for user 2")
	require.True(t, wasCreated2, "User 2 should have been created") // This is the key assertion for the bug fix
	require.NotNil(t, sharedUser2, "Shared user 2 should not be nil")

	// Retrieve user2 from DB and verify ProviderID and AuthProvider
	dbUser2, errDb2 := userRepo.FindByFirebaseUID(ctx, token2.UID)
	require.NoError(t, errDb2, "Failed to find user 2 by Firebase UID")
	require.NotNil(t, dbUser2, "DB User 2 should not be nil")
	assert.Nil(t, dbUser2.ProviderID, "User 2 ProviderID should be nil")
	assert.Equal(t, "firebase", dbUser2.AuthProvider, "User 2 AuthProvider should be 'firebase'")
	assert.Equal(t, token2.UID, *dbUser2.FirebaseUID, "User 2 FirebaseUID should match")
	expectedEmail2 := "user2@test.com"
	assert.Equal(t, expectedEmail2, *dbUser2.Email, "User 2 Email should match")

	// Ensure users are distinct
	assert.NotEqual(t, dbUser1.ID, dbUser2.ID, "User 1 and User 2 should have different IDs")
}

// TestMain could be used for global setup/teardown if needed,
// but for SQLite in-memory, per-test setup is often sufficient.
func TestMain(m *testing.M) {
	// Example: Global setup like initializing something once
	exitVal := m.Run()
	// Example: Global teardown
	os.Exit(exitVal)
}

// Additional test cases can be added here:
// - TestGetOrCreateUser_ExistingUser_ProfileUpdate: Test profile update logic.
// - TestGetOrCreateUser_NewUser_MinimalClaims: Test with minimal claims.
// - TestGetOrCreateUser_EmailConflict: (If applicable, though GetOrCreate usually handles this by finding existing)

// Helper to check if migrations ran (example, might not be directly used if AutoMigrate is trusted)
func checkMigrations(db *gorm.DB, t *testing.T) {
	// Check if 'users' table exists
	if !db.Migrator().HasTable(&user.User{}) {
		t.Fatal("users table does not exist after migration")
	}
	// Add more checks if necessary, e.g., specific columns
	if !db.Migrator().HasColumn(&user.User{}, "firebase_uid") {
		t.Fatal("users table is missing firebase_uid column")
	}
	if !db.Migrator().HasColumn(&user.User{}, "provider_id") {
		t.Fatal("users table is missing provider_id column")
	}
	if !db.Migrator().HasColumn(&user.User{}, "auth_provider") {
		t.Fatal("users table is missing auth_provider column")
	}
}

// Note: The actual GORM model for user.User is expected to be in 'internal/user/user.go'.
// It should look something like this (simplified):
/*
package user

import (
	"time"
	"github.com/google/uuid"
	"seattle_info_backend/internal/common"
)

type User struct {
	common.BaseModel
	FirebaseUID       *string    `gorm:"column:firebase_uid;type:varchar(255);uniqueIndex"`
	Email             *string    `gorm:"column:email;type:varchar(255);uniqueIndex"` // Ensure this handles NULL for uniqueness if multiple users can have NULL email
	IsEmailVerified   bool       `gorm:"column:is_email_verified;default:false"`
	PhoneNumber       *string    `gorm:"column:phone_number;type:varchar(50);uniqueIndex"`
	FirstName         *string    `gorm:"column:first_name;type:varchar(100)"`
	LastName          *string    `gorm:"column:last_name;type:varchar(100)"`
	ProfilePictureURL *string    `gorm:"column:profile_picture_url;type:text"`
	AuthProvider      string     `gorm:"column:auth_provider;type:varchar(50);not null;default:'default'"` // e.g., 'firebase', 'custom'
	ProviderID        *string    `gorm:"column:provider_id;type:varchar(255);index"` // e.g., 'google.com', 'facebook.com', specific ID from IdP. Made nullable.
	Role              string     `gorm:"column:role;type:varchar(50);not null;default:'user'"`
	LastLoginAt       *time.Time `gorm:"column:last_login_at"`
	StripeCustomerID  *string    `gorm:"column:stripe_customer_id;type:varchar(255);uniqueIndex"`
	// Other fields like IsActive, etc.
}
*/
// The `database.Migrate` function would typically call `db.AutoMigrate(&User{})`
// For this test, direct AutoMigrate is used in setup.

// Ensure the `user.User` struct has `ProviderID *string` and `AuthProvider string`.
// The change in `user/service.go` was to stop populating `ProviderID` for Firebase users.
// This test verifies that behavior for new users.
// If `ProviderID` had a NOT NULL constraint AND no default, creation would fail if it's not set.
// However, it's a `*string` (pointer), so `nil` is acceptable.
// The original bug was likely related to a `UNIQUE` constraint on `ProviderID` or a composite key involving it,
// which would fail if multiple users had the same `signInProvider` (e.g., "google.com") assigned to `ProviderID`.
// By setting it to `nil`, this potential unique constraint violation is avoided for new Firebase users.
// `AuthProvider` is set to "firebase" as a general identifier.
// `FirebaseUID` is the actual unique identifier from Firebase.
