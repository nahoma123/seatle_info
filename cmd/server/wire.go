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
	"seattle_info_backend/internal/firebase" // Added
	"seattle_info_backend/internal/jobs"
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/platform/database"
	"seattle_info_backend/internal/platform/logger"
	"seattle_info_backend/internal/shared"
	"seattle_info_backend/internal/user"

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
		// provideCleanup, // This should be fine

		// Firebase Service (New)
		firebase.NewFirebaseService,

		// Core User Services (Adjusted)
		user.NewGORMRepository, // Provides user.Repository
		user.NewService,        // Provides *user.ServiceImplementation (constructor changed)
		wire.Bind(new(shared.Service), new(*user.ServiceImplementation)), // This binding is still key

		// Handlers (Adjusted)
		// auth.NewHandler needs shared.Service (constructor changed)
		auth.NewHandler,
		user.NewHandler, // Needs shared.Service

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
