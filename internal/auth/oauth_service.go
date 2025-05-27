// File: internal/auth/oauth_service.go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"seattle_info_backend/internal/common"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/shared"
	// "time" // Removed unused import, will confirm after other changes

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// OAuthProvider represents an OAuth provider type.
type OAuthProvider string

const (
	ProviderGoogle OAuthProvider = "google"
	ProviderApple  OAuthProvider = "apple"
)

// OAuthService defines the interface for OAuth operations.
type OAuthService interface {
	GetOAuthConfig(provider OAuthProvider) (*oauth2.Config, error)
	GetOAuthCallbackURL(provider OAuthProvider) string
	GetOAuthRedirectURL(provider OAuthProvider, state string) string
	ProcessOAuthCallback(provider OAuthProvider, code string, state string) (string, error)
	GetOAuthProfile(provider OAuthProvider, token string) (*shared.OAuthUserProfile, error)
	GenerateOAuthToken(provider OAuthProvider) (string, error)
	GetGoogleLoginURL(c *gin.Context) (string, error)
	HandleGoogleCallback(c *gin.Context, code string, state string) (*shared.User, *shared.TokenResponse, error) // Changed to shared.TokenResponse
	GetAppleLoginURL(c *gin.Context) (string, error)
	HandleAppleCallback(c *gin.Context, code string, idTokenStr string, state string, appleUserJSON string) (*shared.User, *shared.TokenResponse, error) // Changed to shared.TokenResponse
}

type oauthService struct {
	cfg               *config.Config
	oauthUserProvider OAuthUserProvider      // This is auth.OAuthUserProvider
	tokenService      shared.TokenService  // Changed to shared.TokenService
	logger            *zap.Logger
}

// NewOAuthService creates a new OAuth service.
func NewOAuthService(
	cfg *config.Config,
	oauthUserProvider OAuthUserProvider, // This is auth.OAuthUserProvider
	tokenService shared.TokenService, // Changed to shared.TokenService
	logger *zap.Logger,
) OAuthService { // Return the interface type
	return &oauthService{
		cfg:               cfg,
		oauthUserProvider: oauthUserProvider,
		tokenService:      tokenService,
		logger:            logger.Named("OAuthService"),
	}
}

// GetGoogleLoginURL generates the URL for Google OAuth login.
func (s *oauthService) GetGoogleLoginURL(c *gin.Context) (string, error) {
	state, err := generateAndSetOAuthState(c, s.cfg)
	if err != nil {
		s.logger.Error("Failed to generate OAuth state for Google", zap.Error(err))
		return "", common.ErrInternalServer.WithDetails("Could not initiate Google login.")
	}
	googleCfg := getGoogleOAuthConfig(s.cfg)
	authURL := googleCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	s.logger.Info("Generated Google login URL", zap.String("url", authURL))
	return authURL, nil
}

// HandleGoogleCallback processes the callback from Google.
func (s *oauthService) HandleGoogleCallback(c *gin.Context, code string, state string) (*shared.User, *shared.TokenResponse, error) {
	storedState, err := getOAuthCookie(c, s.cfg, s.cfg.OAuthStateCookieName)
	if err != nil {
		s.logger.Error("Failed to get stored OAuth state for Google callback", zap.Error(err))
		return nil, nil, common.ErrBadRequest.WithDetails("Invalid session or state mismatch.")
	}
	if state != storedState {
		s.logger.Error("Google OAuth state mismatch", zap.String("received_state", state), zap.String("stored_state", storedState))
		return nil, nil, common.ErrBadRequest.WithDetails("OAuth state mismatch. Possible CSRF attack.")
	}

	googleCfg := getGoogleOAuthConfig(s.cfg)
	ctx := context.WithValue(c.Request.Context(), oauth2.HTTPClient, http.DefaultClient)

	token, err := googleCfg.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("Failed to exchange Google auth code for token", zap.Error(err))
		return nil, nil, common.ErrServiceUnavailable.WithDetails("Could not exchange Google auth code.")
	}
	if !token.Valid() {
		s.logger.Error("Google token received is invalid", zap.Any("token", token))
		return nil, nil, common.ErrServiceUnavailable.WithDetails("Received invalid token from Google.")
	}

	client := googleCfg.Client(ctx, token)
	userInfoResp, err := client.Get(googleUserInfoURL)
	if err != nil {
		s.logger.Error("Failed to fetch user info from Google", zap.Error(err))
		return nil, nil, common.ErrServiceUnavailable.WithDetails("Could not fetch user info from Google.")
	}
	defer userInfoResp.Body.Close()

	if userInfoResp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(userInfoResp.Body)
		s.logger.Error("Google user info request failed", zap.Int("status", userInfoResp.StatusCode), zap.String("body", string(bodyBytes)))
		return nil, nil, common.ErrServiceUnavailable.WithDetails(fmt.Sprintf("Google returned status %d for user info.", userInfoResp.StatusCode))
	}

	var googleUser struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(userInfoResp.Body).Decode(&googleUser); err != nil {
		s.logger.Error("Failed to decode Google user info", zap.Error(err))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not process Google user information.")
	}

	userProfile := shared.OAuthUserProfile{
		Provider:      string(ProviderGoogle),
		ProviderID:    googleUser.Sub,
		Email:         strings.ToLower(googleUser.Email),
		FirstName:     googleUser.GivenName,
		LastName:      googleUser.FamilyName,
		PictureURL:    googleUser.Picture,
		EmailVerified: googleUser.EmailVerified,
	}

	// Use the interface method
	appUser, _, err := s.oauthUserProvider.FindOrCreateOrLinkOAuthUser(c.Request.Context(), userProfile)
	if err != nil {
		s.logger.Error("Failed to find or create user from Google profile", zap.Error(err))
		if _, ok := common.IsAPIError(err); ok {
			return nil, nil, err
		}
		return nil, nil, common.ErrInternalServer.WithDetails("Failed to process user account after Google login.")
	}

	accessToken, accessExpiresAt, err := s.tokenService.GenerateAccessToken(appUser)
	if err != nil {
		s.logger.Error("Failed to generate access token after Google login", zap.Error(err), zap.String("userID", appUser.ID.String()))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not generate access token.")
	}
	refreshToken, _, err := s.tokenService.GenerateRefreshToken(appUser)
	if err != nil {
		s.logger.Error("Failed to generate refresh token after Google login", zap.Error(err), zap.String("userID", appUser.ID.String()))
	}

	tokenResponse := &shared.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}

	s.logger.Info("Google OAuth login successful", zap.String("userID", appUser.ID.String()), zap.Stringp("email", appUser.Email))
	return appUser, tokenResponse, nil
}

// GetAppleLoginURL generates the URL for Apple OAuth login.
func (s *oauthService) GetAppleLoginURL(c *gin.Context) (string, error) {
	state, err := generateAndSetOAuthState(c, s.cfg)
	if err != nil {
		s.logger.Error("Failed to generate OAuth state for Apple", zap.Error(err))
		return "", common.ErrInternalServer.WithDetails("Could not initiate Apple login.")
	}
	nonce, err := generateAndSetOAuthNonce(c, s.cfg)
	if err != nil {
		s.logger.Error("Failed to generate OAuth nonce for Apple", zap.Error(err))
		return "", common.ErrInternalServer.WithDetails("Could not initiate Apple login.")
	}

	params := url.Values{}
	params.Add("client_id", s.cfg.AppleClientID)
	params.Add("redirect_uri", s.cfg.AppleRedirectURI)
	params.Add("response_type", "code id_token")
	params.Add("scope", "name email")
	params.Add("response_mode", "form_post")
	params.Add("state", state)
	params.Add("nonce", nonce)

	authURL := appleAuthURL + "?" + params.Encode()
	s.logger.Info("Generated Apple login URL", zap.String("url", authURL))
	return authURL, nil
}

// HandleAppleCallback processes the callback from Apple.
func (s *oauthService) HandleAppleCallback(c *gin.Context, code string, idTokenStr string, state string, appleUserJSON string) (*shared.User, *shared.TokenResponse, error) {
	storedState, err := getOAuthCookie(c, s.cfg, s.cfg.OAuthStateCookieName)
	if err != nil {
		s.logger.Error("Failed to get stored OAuth state for Apple callback", zap.Error(err))
		return nil, nil, common.ErrBadRequest.WithDetails("Invalid session or state mismatch.")
	}
	if state != storedState {
		s.logger.Error("Apple OAuth state mismatch", zap.String("received_state", state), zap.String("stored_state", storedState))
		return nil, nil, common.ErrBadRequest.WithDetails("OAuth state mismatch. Possible CSRF attack.")
	}

	storedNonce, err := getOAuthCookie(c, s.cfg, s.cfg.OAuthNonceCookieName)
	if err != nil {
		s.logger.Error("Failed to get stored OAuth nonce for Apple callback", zap.Error(err))
		return nil, nil, common.ErrBadRequest.WithDetails("Invalid session or nonce missing.")
	}

	appleClaims, err := verifyAppleIDToken(idTokenStr, s.cfg.AppleClientID, storedNonce)
	if err != nil {
		s.logger.Error("Apple ID token verification failed", zap.Error(err))
		return nil, nil, common.ErrUnauthorized.WithDetails("Invalid Apple ID token: " + err.Error())
	}
	s.logger.Info("Apple ID token successfully validated", zap.Any("claims", appleClaims))

	userProfile := shared.OAuthUserProfile{
		Provider:      string(ProviderApple),
		ProviderID:    appleClaims.Subject,
		Email:         strings.ToLower(appleClaims.Email),
		EmailVerified: appleClaims.EmailVerified == "true" || appleClaims.EmailVerified == "TRUE", // Handle both string "true" and boolean true if possible
	}

	if appleUserJSON != "" {
		s.logger.Info("Received Apple user form data", zap.String("user_json", appleUserJSON))
		var appleUserFormData AppleUserForm
		if err := json.Unmarshal([]byte(appleUserJSON), &appleUserFormData); err == nil {
			userProfile.FirstName = appleUserFormData.Name.FirstName
			userProfile.LastName = appleUserFormData.Name.LastName
			if userProfile.Email == "" && appleUserFormData.Email != "" {
				userProfile.Email = strings.ToLower(appleUserFormData.Email)
			}
		} else {
			s.logger.Warn("Failed to parse Apple user form data JSON", zap.Error(err), zap.String("raw_json", appleUserJSON))
		}
	}

	if userProfile.ProviderID == "" {
		s.logger.Error("Apple user subject (provider ID) is missing from ID token")
		return nil, nil, common.ErrBadRequest.WithDetails("Missing user identifier from Apple.")
	}

	// Use the interface method
	appUser, _, err := s.oauthUserProvider.FindOrCreateOrLinkOAuthUser(c.Request.Context(), userProfile)
	if err != nil {
		s.logger.Error("Failed to find or create user from Apple profile", zap.Error(err))
		if _, ok := common.IsAPIError(err); ok {
			return nil, nil, err
		}
		return nil, nil, common.ErrInternalServer.WithDetails("Failed to process user account after Apple login.")
	}

	accessToken, accessExpiresAt, err := s.tokenService.GenerateAccessToken(appUser)
	if err != nil {
		s.logger.Error("Failed to generate access token after Apple login", zap.Error(err), zap.String("userID", appUser.ID.String()))
		return nil, nil, common.ErrInternalServer.WithDetails("Could not generate access token.")
	}
	refreshToken, _, err := s.tokenService.GenerateRefreshToken(appUser)
	if err != nil {
		s.logger.Error("Failed to generate refresh token after Apple login", zap.Error(err), zap.String("userID", appUser.ID.String()))
	}

	tokenResponse := &shared.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    accessExpiresAt,
	}

	s.logger.Info("Apple OAuth login successful", zap.String("userID", appUser.ID.String()), zap.Stringp("email", appUser.Email))
	return appUser, tokenResponse, nil
}

// GenerateOAuthToken is a placeholder implementation for the OAuthService interface.
func (s *oauthService) GenerateOAuthToken(provider OAuthProvider) (string, error) {
	s.logger.Info("GenerateOAuthToken called (placeholder)", zap.String("provider", string(provider)))
	// In a real scenario, this would involve more complex logic to generate or retrieve
	// a token suitable for server-to-server communication or specific OAuth flows.
	// For now, returning a dummy token and no error.
	return "dummy-oauth-token-for-" + string(provider), nil
}

// GetOAuthConfig is a placeholder implementation
func (s *oauthService) GetOAuthConfig(provider OAuthProvider) (*oauth2.Config, error) {
    // TODO: Implement actual logic for different providers
    s.logger.Info("GetOAuthConfig called (placeholder)", zap.String("provider", string(provider)))
    if provider == ProviderGoogle {
        return getGoogleOAuthConfig(s.cfg), nil
    }
    // Add other providers like Apple if they use a standard oauth2.Config flow here
    return nil, fmt.Errorf("OAuth provider %s configuration not implemented", provider)
}

// GetOAuthCallbackURL is a placeholder implementation
func (s *oauthService) GetOAuthCallbackURL(provider OAuthProvider) string {
    // TODO: Implement actual logic
    s.logger.Info("GetOAuthCallbackURL called (placeholder)", zap.String("provider", string(provider)))
    if provider == ProviderGoogle {
        return s.cfg.GoogleRedirectURI
    }
    if provider == ProviderApple {
        return s.cfg.AppleRedirectURI
    }
    return ""
}

// GetOAuthRedirectURL is a placeholder implementation
func (s *oauthService) GetOAuthRedirectURL(provider OAuthProvider, state string) string {
    // TODO: Implement actual logic
    s.logger.Info("GetOAuthRedirectURL called (placeholder)", zap.String("provider", string(provider)), zap.String("state", state))
    // Example for Google:
    // googleCfg := getGoogleOAuthConfig(s.cfg)
    // return googleCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
    return "http://localhost/dummy-redirect?state=" + state
}

// ProcessOAuthCallback is a placeholder implementation
func (s *oauthService) ProcessOAuthCallback(provider OAuthProvider, code string, state string) (string, error) {
    // TODO: Implement actual logic (e.g., exchange code for token)
    s.logger.Info("ProcessOAuthCallback called (placeholder)", zap.String("provider", string(provider)))
    return "dummy-processed-token-for-" + string(provider), nil
}

// GetOAuthProfile is a placeholder implementation
func (s *oauthService) GetOAuthProfile(provider OAuthProvider, token string) (*shared.OAuthUserProfile, error) {
    // TODO: Implement actual logic
    s.logger.Info("GetOAuthProfile called (placeholder)", zap.String("provider", string(provider)))
    return &shared.OAuthUserProfile{Provider: string(provider), ProviderID: "dummy-id"}, nil
}
