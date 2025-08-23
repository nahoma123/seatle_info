// File: internal/common/context_keys.go
package common

const (
	// AuthorizationHeader is the header name for authorization token
	AuthorizationHeader = "Authorization"
	// AuthorizationTypeBearer is the prefix for Bearer tokens
	AuthorizationTypeBearer = "Bearer"
	// UserIDKey is the context key for storing the authenticated user's ID
	UserIDKey = "userID"
	// UserEmailKey is the context key for storing the authenticated user's email
	UserEmailKey = "userEmail"
	// UserRoleKey is the context key for storing the authenticated user's role
	UserRoleKey = "userRole"
	// FirebaseUIDKey is the context key for storing the Firebase UID
	FirebaseUIDKey = "firebaseUID"
)
