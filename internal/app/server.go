// File: internal/app/server.go
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"seattle_info_backend/internal/auth"
	// "seattle_info_backend/internal/auth" // Duplicate import removed
	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/common" // Added for common.RoleAdmin
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/firebase"
	"seattle_info_backend/internal/jobs"
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/middleware"
	"seattle_info_backend/internal/shared"
	"seattle_info_backend/internal/user"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm" // Added for db *gorm.DB parameter
)

// Server struct holds the dependencies for the HTTP server.
type Server struct {
	httpServer *http.Server
	router     *gin.Engine
	cfg        *config.Config
	logger     *zap.Logger
	// firebaseService *firebase.FirebaseService // Stored if needed by other methods
	// userService shared.Service // Stored if needed by other methods

	// Handlers
	userHandler     *user.Handler
	authHandler     *auth.Handler
	categoryHandler *category.Handler
	listingHandler  *listing.Handler

	// Jobs
	listingExpiryJob *jobs.ListingExpiryJob

	// Middleware instances
	authMW      gin.HandlerFunc
	adminRoleMW gin.HandlerFunc
}

// NewServer creates a new instance of our application server.
func NewServer(
	cfg *config.Config,
	logger *zap.Logger,
	userHandler *user.Handler,
	authHandler *auth.Handler,
	categoryHandler *category.Handler,
	listingHandler *listing.Handler,
	listingExpiryJob *jobs.ListingExpiryJob,
	db *gorm.DB, // Added db *gorm.DB
	firebaseService *firebase.FirebaseService,
	userService shared.Service,
) (*Server, error) {
	gin.SetMode(cfg.GinMode)
	router := gin.New()

	// --- Global Middleware ---
	router.Use(middleware.ZapLogger(logger, cfg))
	router.Use(middleware.ErrorHandler(logger))
	router.Use(gin.Recovery())

	// CORS Middleware
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", middleware.RequestIDHeader}
	corsConfig.AllowCredentials = true
	corsConfig.ExposeHeaders = []string{"Content-Length", middleware.RequestIDHeader}
	router.Use(cors.New(corsConfig))

	// Create middleware instances
	authMW := middleware.AuthMiddleware(firebaseService, userService, logger.Named("AuthMiddleware"))
	adminRoleMW := middleware.RoleAuthMiddleware(common.RoleAdmin) // Use common.RoleAdmin

	// --- Setup Routes ---
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "message": "Seattle Info API is healthy!"})
	})

	v1 := router.Group("/api/v1")

	// Register auth routes (e.g., /auth/me)
	// These routes will be under /api/v1/auth and will use the authMW
	authRouterGroup := v1.Group("/auth", authMW)
	authHandler.RegisterRoutes(authRouterGroup)

	// Register user routes (e.g., /users/me, /users/{id})
	// Some routes inside userHandler might be public, some private.
	// userHandler.RegisterRoutes should handle applying authMW selectively or expect a pre-configured group.
	// For now, assuming userHandler.RegisterRoutes is adapted to take a group that already has authMW for its private routes.
	// And any public routes it has are registered on a different group or handled internally.
	// Let's pass the v1 group and authMW, allowing RegisterRoutes to decide.
	// This means userHandler.RegisterRoutes signature in internal/user/handler.go might need to be
	// func (h *Handler) RegisterRoutes(router *gin.RouterGroup, authMiddleware gin.HandlerFunc)
	// Or, more simply, pass the authenticated group:
	userRouterGroup := v1.Group("/users") // Base for all /users
	userHandler.RegisterRoutes(userRouterGroup, authMW) // Pass authMW for userHandler to apply to its private routes

	// Public GET routes for categories and listings
	// Assuming RegisterPublicRoutes exist and take *gin.RouterGroup
	publicCategoriesGroup := v1.Group("/categories")
	categoryHandler.RegisterPublicRoutes(publicCategoriesGroup)

	publicListingsGroup := v1.Group("/listings")
	listingHandler.RegisterPublicRoutes(publicListingsGroup)

	// Authenticated and Admin specific routes for categories and listings
	// These groups already have authMW and then adminRoleMW applied
	adminCategoriesGroup := v1.Group("/categories", authMW, adminRoleMW)
	categoryHandler.RegisterAdminRoutes(adminCategoriesGroup)

	adminListingsGroup := v1.Group("/listings", authMW, adminRoleMW)
	listingHandler.RegisterAdminRoutes(adminListingsGroup)

	// Authenticated (non-admin) routes for listings (e.g., creating one's own listing)
	// This group has authMW applied
	authenticatedListingsGroup := v1.Group("/listings", authMW)
	listingHandler.RegisterAuthenticatedRoutes(authenticatedListingsGroup)


	addr := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpServer:       httpServer,
		router:           router,
		cfg:              cfg,
		logger:           logger,
		userHandler:      userHandler,
		authHandler:      authHandler,
		categoryHandler:  categoryHandler,
		listingHandler:   listingHandler,
		listingExpiryJob: listingExpiryJob,
		authMW:           authMW,
		adminRoleMW:      adminRoleMW,
		// firebaseService: firebaseService, // Store if needed elsewhere
		// userService: userService,
	}, nil
}

func (s *Server) Start() error {
	if s.listingExpiryJob != nil {
		err := s.listingExpiryJob.SetupAndStart()
		if err != nil {
			s.logger.Error("Failed to setup and start listing expiry job", zap.Error(err))
		}
	} else {
		s.logger.Info("Listing expiry job is not configured, skipping start.")
	}

	s.logger.Info("HTTP Server starting",
		zap.String("address", s.httpServer.Addr),
		zap.String("gin_mode", s.cfg.GinMode),
	)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Error("Failed to start HTTP server", zap.Error(err))
		return err
	}
	s.logger.Info("HTTP Server stopped gracefully or an error occurred")
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Attempting graceful server shutdown...")
	if s.listingExpiryJob != nil {
		s.listingExpiryJob.Stop()
	}
	return s.httpServer.Shutdown(ctx)
}
