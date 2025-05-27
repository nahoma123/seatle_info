// File: internal/auth/handler.go
package auth

import (
	"errors"
	"fmt"      // For fmt.Sprintf in callback logging
	"net/http" // For http.StatusTemporaryRedirect

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/shared"
	"seattle_info_backend/internal/user" // Added import for user.ToUserResponse

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// Handler struct holds dependencies for auth handlers.
type Handler struct {
	userService  shared.Service // Interface type
	tokenService shared.TokenService // Interface type
	oauthService OAuthService // Interface type
	logger       *zap.Logger
}

// NewHandler creates a new auth handler.
func NewHandler(
	userService shared.Service,
	tokenService shared.TokenService,
	oauthService OAuthService,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		userService:  userService,
		tokenService: tokenService,
		oauthService: oauthService,
		logger:       logger,
	}
}

// RegisterRoutes sets up the routes for authentication operations.
func (h *Handler) RegisterRoutes(router *gin.RouterGroup) {
	authGroup := router.Group("/auth")
	{
		authGroup.POST("/login", h.login)
		authGroup.POST("/refresh-token", h.refreshToken)
		authGroup.GET("/google/login", h.googleLogin)
		authGroup.GET("/google/callback", h.googleCallback)
		authGroup.GET("/apple/login", h.appleLogin)
		authGroup.POST("/apple/callback", h.appleCallback)
	}
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Login: Invalid request body", zap.Error(err))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}

	loggedInUser, tokenResponse, err := h.userService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}

	response := gin.H{
		"user":  user.ToUserResponse(loggedInUser), // user.ToUserResponse is fine here
		"token": tokenResponse,
	}
	common.RespondOK(c, "Login successful.", response)
}

func (h *Handler) refreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Refresh token: Invalid request body", zap.Error(err))
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			common.RespondWithError(c, common.NewValidationAPIError(common.FormatValidationErrors(ve)))
			return
		}
		common.RespondWithError(c, common.ErrBadRequest.WithDetails(err.Error()))
		return
	}

	claims, err := h.tokenService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		h.logger.Warn("Refresh token validation failed", zap.Error(err), zap.String("token", req.RefreshToken))
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Invalid or expired refresh token."))
		return
	}

	// Here we use userService.GetUserByID, which is part of auth.OAuthUserProvider interface as well
	u, err := h.userService.GetUserByID(c.Request.Context(), claims.UserID)
	if err != nil {
		h.logger.Error("User not found for valid refresh token claims", zap.String("userID", claims.UserID.String()), zap.Error(err))
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("User associated with refresh token not found."))
		return
	}

	newAccessToken, newAccessExpiresAt, err := h.tokenService.GenerateAccessToken(u)
	if err != nil {
		h.logger.Error("Failed to generate new access token during refresh", zap.Error(err), zap.String("userID", u.ID.String()))
		common.RespondWithError(c, common.ErrInternalServer.WithDetails("Could not generate new access token."))
		return
	}

	newTokenResponse := &shared.TokenResponse{ // Changed to shared.TokenResponse
		AccessToken:  newAccessToken,
		RefreshToken: req.RefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    newAccessExpiresAt,
	}
	common.RespondOK(c, "Token refreshed successfully.", newTokenResponse)
}

func (h *Handler) googleLogin(c *gin.Context) {
	authURL, err := h.oauthService.GetGoogleLoginURL(c)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (h *Handler) googleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if errorParam := c.Query("error"); errorParam != "" {
		errorDesc := c.Query("error_description")
		h.logger.Error("Google OAuth callback error", zap.String("error", errorParam), zap.String("description", errorDesc))
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Google login failed: "+errorDesc))
		return
	}

	if code == "" || state == "" {
		h.logger.Warn("Google callback missing code or state")
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Missing authorization code or state from Google."))
		return
	}

	appUser, tokenResponse, err := h.oauthService.HandleGoogleCallback(c, code, state)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}

	response := gin.H{
		"message": "Google login successful. Use these tokens for API access.",
		"user":    user.ToUserResponse(appUser),
		"token":   tokenResponse,
	}
	common.RespondOK(c, "Google login processed successfully.", response)
}

func (h *Handler) appleLogin(c *gin.Context) {
	authURL, err := h.oauthService.GetAppleLoginURL(c)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (h *Handler) appleCallback(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		h.logger.Error("Failed to parse form for Apple callback", zap.Error(err))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Could not parse Apple callback form."))
		return
	}

	code := c.PostForm("code")
	idToken := c.PostForm("id_token")
	state := c.PostForm("state")
	userJSON := c.PostForm("user")

	if errorParam := c.PostForm("error"); errorParam != "" {
		h.logger.Error("Apple OAuth callback error", zap.String("error", errorParam))
		common.RespondWithError(c, common.ErrUnauthorized.WithDetails("Apple Sign-In failed: "+errorParam))
		return
	}

	if idToken == "" || state == "" {
		h.logger.Warn("Apple callback missing id_token or state", zap.String("code", code), zap.String("id_token_present", fmt.Sprintf("%t", idToken != "")), zap.String("state_present", fmt.Sprintf("%t", state != "")))
		common.RespondWithError(c, common.ErrBadRequest.WithDetails("Missing id_token or state from Apple."))
		return
	}

	appUser, tokenResponse, err := h.oauthService.HandleAppleCallback(c, code, idToken, state, userJSON)
	if err != nil {
		common.RespondWithError(c, err)
		return
	}

	response := gin.H{
		"message": "Apple Sign-In successful. Use these tokens for API access.",
		"user":    user.ToUserResponse(appUser),
		"token":   tokenResponse,
	}
	common.RespondOK(c, "Apple Sign-In processed successfully.", response)
}
