// File: internal/auth/blocklist.go
package auth

import (
	"context"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// TokenBlocklistService defines the interface for a JWT blocklist.
type TokenBlocklistService interface {
	// AddToBlocklist adds a token's JTI (JWT ID) to the blocklist with a given expiration.
	AddToBlocklist(ctx context.Context, jti string, expiresAt time.Time) error
	// IsBlocklisted checks if a token's JTI is in the blocklist.
	IsBlocklisted(ctx context.Context, jti string) (bool, error)
}

// InMemoryBlocklistService is an in-memory implementation of TokenBlocklistService using a cache.
type InMemoryBlocklistService struct {
	mu    sync.RWMutex
	cache *cache.Cache
}

// InMemoryBlocklistConfig holds the configuration for the InMemoryBlocklistService.
type InMemoryBlocklistConfig struct {
	DefaultExpiration time.Duration
	CleanupInterval   time.Duration
}

// NewInMemoryBlocklistService creates a new in-memory blocklist service.
func NewInMemoryBlocklistService(cfg InMemoryBlocklistConfig) *InMemoryBlocklistService {
	return &InMemoryBlocklistService{
		cache: cache.New(cfg.DefaultExpiration, cfg.CleanupInterval),
	}
}

// AddToBlocklist adds a token JTI to the in-memory cache.
// The item will be automatically removed from the cache after it expires.
func (s *InMemoryBlocklistService) AddToBlocklist(ctx context.Context, jti string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate the remaining duration until the token expires.
	// This ensures the JTI is blocklisted for exactly as long as the token would have been valid.
	duration := time.Until(expiresAt)

	// If the token is already expired, we don't need to add it to the blocklist.
	if duration <= 0 {
		return nil
	}

	s.cache.Set(jti, true, duration)
	return nil
}

// IsBlocklisted checks if a token JTI exists in the in-memory cache.
func (s *InMemoryBlocklistService) IsBlocklisted(ctx context.Context, jti string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, found := s.cache.Get(jti)
	return found, nil
}
