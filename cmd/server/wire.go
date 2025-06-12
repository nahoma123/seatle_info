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
	"seattle_info_backend/internal/notification" // Add this
	"seattle_info_backend/internal/platform/database"
	platformElasticsearch "seattle_info_backend/internal/platform/elasticsearch" // Re-add
	"seattle_info_backend/internal/platform/logger"
	"seattle_info_backend/internal/shared"
	"seattle_info_backend/internal/user"

	elasticsearchbase "github.com/elastic/go-elasticsearch/v8" // Re-add with alias
	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Dummy variables
var _ *platformElasticsearch.ESClientWrapper
var _ *elasticsearchbase.Client

// initializeServer is the main Wire injector.
// Signature reverted to (*app.Server, func(), error) as ESClient & Logger are now in Server struct
func initializeServer(cfg *config.Config) (*app.Server, func(), error) {
	wire.Build(
		// Platform Layer
		logger.New, // Stays, needed by app.NewServer (and other services)
		database.NewGORM,
		platformElasticsearch.NewClient, // Re-add
		// provideCleanup is NOT in Build; Wire aggregates cleanup.

		// Firebase Service (New)
		firebase.NewFirebaseService,

		// Core User Services
		user.NewGORMRepository, // Returns user.Repository
		user.NewService,        // Returns *user.ServiceImplementation
		wire.Bind(new(shared.Service), new(*user.ServiceImplementation)), // Binds *user.ServiceImplementation to shared.Service interface

		// Auth Handler (depends on shared.Service and firebase.Service)
		auth.NewHandler,

		// User Handler (depends on shared.Service)
		user.NewHandler,

		// Category Module
		category.NewGORMRepository, // Returns category.Repository
		category.NewService,        // Returns category.Service (interface)
		// No bind needed for category.Service as NewService returns the interface.
		// wire.Bind(new(category.Service), new(*category.ServiceImplementation)), // REMOVED
		category.NewHandler,

		// Notification Module
		notification.NewGORMRepository, // Returns notification.Repository
		// No bind needed for notification.Repository as NewGORMRepository returns the interface.
		// wire.Bind(new(notification.Repository), new(*notification.GORMRepository)), // REMOVED
		notification.NewService, // Returns notification.Service (interface)
		// No bind needed for notification.Service as NewService returns the interface.
		// wire.Bind(new(notification.Service), new(*notification.ServiceImplementation)), // REMOVED
		notification.NewHandler,

		// Listing Module (listing.NewService depends on notification.Service)
		listing.NewGORMRepository, // Returns listing.Repository
		// No bind needed for listing.Repository as NewGORMRepository returns the interface.
		// wire.Bind(new(listing.Repository), new(*listing.GORMRepository)), // REMOVED
		listing.NewService, // Returns listing.Service (interface)
		// No bind needed for listing.Service as NewService returns the interface.
		// wire.Bind(new(listing.Service), new(*listing.ServiceImplementation)), // REMOVED
		listing.NewHandler,

		jobs.NewListingExpiryJob,

		// Application Layer
		app.NewServer, // app.NewServer now needs ESClient and Logger
	)
	return nil, nil, nil // Match new signature (3 returns)
}

// provideCleanup is defined but NOT included in wire.Build above.
// Wire will generate its own cleanup function.
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
