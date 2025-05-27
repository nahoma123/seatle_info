// File: cmd/server/wire.go
//go:build wireinject
// +build wireinject

package main

import (
	"log"
	"seattle_info_backend/internal/app"
	"seattle_info_backend/internal/auth"
	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/jobs"
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/platform/database"
	"seattle_info_backend/internal/platform/logger"
	"seattle_info_backend/internal/user"
	"seattle_info_backend/internal/shared"

	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// initializeServer is the main Wire injector.
func initializeServer(cfg *config.Config) (*app.Server, func(), error) {
	wire.Build(
		// Platform Layer
		logger.New,
		database.NewGORM,
		// provideCleanup, // Removed to see if Wire infers it from return signature

		// Core Services providing interfaces/concrete types needed by other services
		auth.NewJWTService,     // Now provides *auth.JWTService (concrete)
		wire.Bind(new(shared.TokenService), new(*auth.JWTService)), // Explicitly bind interface to concrete
		user.NewGORMRepository, // Provides user.Repository (needed by user.NewService)

		// Concrete Service Implementations
		// user.NewService depends on user.Repository and shared.TokenService
		user.NewService,        // Provides *user.ServiceImplementation

		// Interface Bindings for *user.ServiceImplementation
		// This makes *user.ServiceImplementation fulfill shared.Service and auth.OAuthUserProvider
		wire.Bind(new(shared.Service), new(*user.ServiceImplementation)),
		wire.Bind(new(auth.OAuthUserProvider), new(*user.ServiceImplementation)),

		// Other Services that depend on the bound interfaces or concrete types
		// auth.NewOAuthService depends on auth.OAuthUserProvider (which is *user.ServiceImplementation)
		auth.NewOAuthService,   // Provides auth.OAuthService

		// Handlers
		// auth.NewHandler needs shared.Service, shared.TokenService, auth.OAuthService
		auth.NewHandler,
		// user.NewHandler needs shared.Service
		user.NewHandler,
		
		// Other Modules
		category.NewGORMRepository,
		category.NewService,
		category.NewHandler,
		listing.NewGORMRepository,
		listing.NewService,
		listing.NewHandler,
		jobs.NewListingExpiryJob,

		// Application Layer
		app.NewServer,
	)
	return nil, nil, nil
}

func provideCleanup(logger *zap.Logger, db *gorm.DB) func() {
	return func() {
		logger.Info("Executing cleanup tasks...")
		database.CloseGORMDB(db)
		if err := logger.Sync(); err != nil {
			log.Printf("ERROR: Failed to sync logger during cleanup: %v", err)
		}
		log.Println("Cleanup finished.")
	}
}
