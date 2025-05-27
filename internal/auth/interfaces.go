// File: internal/auth/interfaces.go
package auth

import (
	"context"

	"github.com/google/uuid"
	"seattle_info_backend/internal/shared"
)

// OAuthUserProvider defines the user operations needed by the OAuthService.
// This interface will be implemented by user.ServiceImplementation.
type OAuthUserProvider interface {
	FindOrCreateOrLinkOAuthUser(ctx context.Context, profile shared.OAuthUserProfile) (usr *shared.User, wasCreated bool, err error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*shared.User, error)
}
