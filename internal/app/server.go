// File: internal/app/server.go
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"seattle_info_backend/internal/auth"
	"seattle_info_backend/internal/category"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/jobs"
	"seattle_info_backend/internal/listing"
	"seattle_info_backend/internal/middleware"
	"seattle_info_backend/internal/shared" // Added missing import
	"seattle_info_backend/internal/user"

	"github.com/gin-contrib/cors" // Import CORS
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server struct holds the dependencies for the HTTP server.
type Server struct {
	httpServer *http.Server
	router     *gin.Engine
	cfg        *config.Config
	logger     *zap.Logger
	// tokenService auth.TokenService // No longer needed directly here if middleware passed

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
	tokenService shared.TokenService, // Changed to shared.TokenService
	userHandler *user.Handler,
	authHandler *auth.Handler,
	categoryHandler *category.Handler,
	listingHandler *listing.Handler,
	listingExpiryJob *jobs.ListingExpiryJob,
) (*Server, error) {
	gin.SetMode(cfg.GinMode)
	router := gin.New()

	// --- Global Middleware ---
	router.Use(middleware.ZapLogger(logger, cfg))
	router.Use(middleware.ErrorHandler(logger))
	router.Use(gin.Recovery())

	// CORS Middleware
	corsConfig := cors.DefaultConfig()
	// Allow all origins for development, restrict in production
	corsConfig.AllowOrigins = []string{"*"} // Replace with frontend URL in prod e.g. "http://localhost:3000", "https://yourdomain.com"
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", middleware.RequestIDHeader}
	corsConfig.AllowCredentials = true // If you need to send cookies or auth headers
	corsConfig.ExposeHeaders = []string{"Content-Length", middleware.RequestIDHeader}
	router.Use(cors.New(corsConfig))

	// Create middleware instances to pass to handlers
	authMW := middleware.AuthMiddleware(tokenService, logger) // tokenService is injected into NewServer
	adminRoleMW := middleware.RoleAuthMiddleware("admin")

	// --- Setup Routes ---
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "message": "Seattle Info API is healthy!"})
	})

	v1 := router.Group("/api/v1")
	{
		// Pass middleware instances to handler registration methods
		userHandler.RegisterRoutes(v1, authMW)
		categoryHandler.RegisterRoutes(v1, authMW, adminRoleMW)
		listingHandler.RegisterRoutes(v1, authMW, adminRoleMW)
		authHandler.RegisterRoutes(v1) // Auth handler itself doesn't typically need auth middleware on its routes
	}

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
		authMW:           authMW, // Store if needed for other dynamic routing
		adminRoleMW:      adminRoleMW,
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
