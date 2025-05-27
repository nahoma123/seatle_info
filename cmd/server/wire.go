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

		// User Module (Provide concrete implementation first, then bind interfaces)
		user.NewGORMRepository, // Provides user.Repository (interface)
		user.NewService,        // Provides *user.ServiceImplementation (concrete type)
		// Bind the concrete *user.ServiceImplementation to the shared.Service interface
		wire.Bind(new(shared.Service), new(*user.ServiceImplementation)),

		// Auth Module
		auth.NewJWTService, // Provides auth.TokenService (interface)
		// Bind JWTService to shared.TokenService
		wire.Bind(new(shared.TokenService), new(*auth.JWTService)),
		// Bind the concrete *user.ServiceImplementation to the auth.OAuthUserProvider interface
		wire.Bind(new(auth.OAuthUserProvider), new(*user.ServiceImplementation)),
		auth.NewOAuthService, // Provides auth.OAuthService (interface)

		// Auth Handler (depends on shared.Service, shared.TokenService, auth.OAuthService)
		auth.NewHandler,

		// User Handler (depends on shared.Service)
		user.NewHandler,

		// Category Module
		category.NewGORMRepository,
		category.NewService,
		// If category.NewService returns an interface, you might need a bind here too, e.g.:
		// wire.Bind(new(category.Service), new(*category.ServiceImplementation)),
		category.NewHandler,

		// Listing Module
		listing.NewGORMRepository,
		listing.NewService,
		// If listing.NewService returns an interface:
		// wire.Bind(new(listing.Service), new(*listing.ServiceImplementation)),
		listing.NewHandler,

		// Jobs Module
		jobs.NewListingExpiryJob,

		// Application Layer
		app.NewServer,

		provideCleanup,
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
