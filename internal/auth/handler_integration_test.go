package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/platform/database"
	"seattle_info_backend/internal/platform/logger"
	"seattle_info_backend/internal/shared" // For shared types if needed in test setup
	"seattle_info_backend/internal/user"
	"testing"
	"time"

	// "seattle_info_backend/internal/app" // May not be needed directly if we build router manually

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json" // For marshalling JWKS

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"gopkg.in/square/go-jose.v2" // For JWK
	josejwt "gopkg.in/square/go-jose.v2/jwt" // For generating JWT
	"gorm.io/gorm"
)

// IntegrationTestSuite holds the suite state.
type IntegrationTestSuite struct {
	suite.Suite
	Router   *gin.Engine
	DB       *gorm.DB
	Cfg      *config.Config
	Logger   *zap.Logger
	UserRepo user.Repository
	UserSvc  shared.Service      // This is the interface user.ServiceImplementation implements
	TokenSvc shared.TokenService // This is the interface auth.JWTService implements
	OAuthSvc OAuthService        // This is the interface auth.oauthService implements
	AuthHdlr *Handler

	// Apple Test Helpers
	testApplePrivKey    *ecdsa.PrivateKey
	testApplePubKeyJWK  jose.JSONWebKey
	mockAppleJWKSServer *httptest.Server
	originalAppleJWKSURL string // To store and restore the original AppleJWKSURL
	// Add other handlers or services if they become part of the common setup
}

// generateTestAppleKeys generates an ECDSA P-256 key pair for Apple tests.
func (s *IntegrationTestSuite) generateTestAppleKeys() {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.Require().NoError(err, "Failed to generate ECDSA private key for Apple test")
	s.testApplePrivKey = privateKey

	// Create a JWK from the public key
	// The KeyID "testkid" is arbitrary for testing purposes.
	s.testApplePubKeyJWK = jose.JSONWebKey{
		Key:       &privateKey.PublicKey,
		KeyID:     "testkid_apple", // Ensure this matches what the dummy token will use
		Algorithm: string(jose.ES256),
		Use:       "sig",
	}
}

// generateDummyAppleIDToken creates a signed JWT for Apple OAuth tests.
func (s *IntegrationTestSuite) generateDummyAppleIDToken(
	audience string, 
	subject string, 
	email string, 
	nonce string, 
	issuedAt time.Time, 
	expiresAt time.Time,
) (string, error) {
	if s.testApplePrivKey == nil {
		return "", errors.New("test Apple private key not generated/set in test suite")
	}

	// Create a signer from the ECDSA private key
	// The KeyID must match the one in the JWK served by the mock JWKS endpoint.
	signerOpts := jose.SignerOptions{}.WithType("JWT").WithHeader("kid", s.testApplePubKeyJWK.KeyID)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: s.testApplePrivKey}, &signerOpts)
	if err != nil {
		return "", fmt.Errorf("failed to create JWT signer: %w", err)
	}

	// Construct claims
	// Note: Apple's `email_verified` and `is_private_email` are often strings "true"/"false"
	claims := josejwt.Claims{
		Issuer:   appleIssuer, // Use the const from auth_helper.go
		Audience: josejwt.Audience{audience}, // Should be s.Cfg.AppleClientID
		Subject:  subject,
		Expiry:   josejwt.NewNumericDate(expiresAt),
		IssuedAt: josejwt.NewNumericDate(issuedAt),
		ID:       "dummy-jwt-id-" + time.Now().String(), // A unique ID for the token
	}
	// Custom claims for Apple
	customClaims := struct {
		Email          string `json:"email,omitempty"`
		EmailVerified  string `json:"email_verified,omitempty"` // Apple sends this as string "true" or "false"
		IsPrivateEmail string `json:"is_private_email,omitempty"`
		Nonce          string `json:"nonce,omitempty"`
		AuthTime       int64  `json:"auth_time,omitempty"`
	}{
		Email:          email,
		EmailVerified:  "true", // Assume verified for most tests
		IsPrivateEmail: "false",
		Nonce:          nonce,
		AuthTime:       issuedAt.Unix(),
	}

	// Build the token
	tokenBuilder := josejwt.Signed(signer).Claims(claims).Claims(customClaims)
	signedToken, err := tokenBuilder.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to sign Apple ID token: %w", err)
	}
	return signedToken, nil
}


func (s *IntegrationTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	s.Logger = logger.New("info", true) // Or zap.NewNop() for less verbose tests

	// Generate Apple test keys first, as they might be needed for config if KID is dynamic
	s.generateTestAppleKeys()

	// Configuration (consider loading from a test-specific file or env vars)
	s.Cfg = &config.Config{
		GinMode:                     gin.TestMode,
		ServerHost:                  "localhost",
		ServerPort:                  "0", // Use "0" for httptest to pick a random port
		JWTSecretKey:                "test-secret-key-very-long-and-secure",
		JWTAccessTokenExpiryMinutes: time.Minute * 15,
		OAuthCookieMaxAgeMinutes:    60,
		OAuthStateCookieName:        "test_oauth_state",
		OAuthNonceCookieName:        "test_oauth_nonce",
		// Database - IMPORTANT: Use a test database connection string
		DBHost:                   os.Getenv("TEST_DB_HOST"), // Read from ENV
		DBPort:                   os.Getenv("TEST_DB_PORT"),
		DBUser:                   os.Getenv("TEST_DB_USER"),
		DBPassword:               os.Getenv("TEST_DB_PASSWORD"),
		DBName:                   os.Getenv("TEST_DB_NAME"),
		DBSSLMode:                "disable", // Typically disable for local test DBs
		DBMaxOpenConns:           10,
		DBMaxIdleConns:           5,
		DBConnMaxLifetimeMinutes: 60,

		// Dummy OAuth creds (replace with actual test creds or use mocks)
		GoogleClientID:      "test-google-client-id",
		GoogleClientSecret:  "test-google-client-secret",
		GoogleRedirectURI:   "http://localhost/api/v1/auth/google/callback", // Test server callback
		AppleClientID:       "test-apple-client-id",
		AppleTeamID:         "test-apple-team-id",
		AppleKeyID:          "test-apple-key-id",
		ApplePrivateKeyPath: "", // Needs careful handling for tests
		AppleRedirectURI:    "http://localhost/api/v1/auth/apple/callback", // Test server callback
	}

	// Fallback to default test DB if ENV vars are not set
	if s.Cfg.DBHost == "" {
		s.Cfg.DBHost = "localhost"
	}
	if s.Cfg.DBPort == "" {
		s.Cfg.DBPort = "5432"
	}
	if s.Cfg.DBUser == "" {
		s.Cfg.DBUser = "testuser"
	}
	if s.Cfg.DBPassword == "" {
		s.Cfg.DBPassword = "testpassword"
	}
	if s.Cfg.DBName == "" {
		s.Cfg.DBName = "testdb"
	}

	var err error
	s.DB, err = database.NewGORM(s.Cfg, s.Logger)
	s.Require().NoError(err, "Failed to connect to test database")

	// Setup services and handlers
	// Note: NewGORMRepository in user package also takes logger. Assuming it's optional or handled.
	// If NewGORMRepository strictly needs logger, it should be passed.
	// Based on wire.go, user.NewGORMRepository takes only *gorm.DB.
	// Let's assume it doesn't need logger for now, or this will be caught in compilation.
	// Forcing it to not take logger for now to match potential simpler signature.
	// If user.NewGORMRepository is `func NewGORMRepository(db *gorm.DB) Repository`
	s.UserRepo = user.NewGORMRepository(s.DB) // Pass s.DB if it takes only DB
	// If it is `func NewGORMRepository(db *gorm.DB, logger *zap.Logger) Repository` use:
	// s.UserRepo = user.NewGORMRepository(s.DB, s.Logger)

	s.TokenSvc = NewJWTService(s.Cfg, s.Logger) // auth.NewJWTService returns *JWTService (concrete), which implements shared.TokenService
	s.UserSvc = user.NewService(s.UserRepo, s.TokenSvc, s.Cfg, s.Logger)

	// Ensure s.UserSvc (which is *user.ServiceImplementation) can be asserted to auth.OAuthUserProvider
	// This relies on *user.ServiceImplementation correctly implementing the methods of auth.OAuthUserProvider
	var oAuthUserProvider OAuthUserProvider
	var ok bool
	oAuthUserProvider, ok = s.UserSvc.(OAuthUserProvider)
	if !ok {
		// This will cause a panic, which is fine for a test setup if the assertion fails.
		// It indicates a mismatch in interface implementation that needs to be fixed.
		s.T().Fatalf("user.ServiceImplementation does not implement auth.OAuthUserProvider")
	}
	s.OAuthSvc = NewOAuthService(s.Cfg, oAuthUserProvider, s.TokenSvc, s.Logger)
	s.AuthHdlr = NewHandler(s.UserSvc, s.TokenSvc, s.OAuthSvc, s.Logger)

	// Setup router
	router := gin.New()
	// Apply global middleware if any (e.g., logger, error handler - simplified for now)
	// router.Use(middleware.ZapLogger(s.Logger, s.Cfg))
	// router.Use(middleware.ErrorHandler(s.Logger))
	apiV1 := router.Group("/api/v1")
	s.AuthHdlr.RegisterRoutes(apiV1) // Register only auth routes for this test suite

	s.Router = router

	// Setup Mock Apple JWKS Server
	s.mockAppleJWKSServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{s.testApplePubKeyJWK},
		}
		jwksBytes, err := json.Marshal(jwks)
		s.Require().NoError(err, "Failed to marshal test Apple JWKS")
		
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(jwksBytes)
		s.Require().NoError(err)
	}))

	// Override AppleJWKSURL for the duration of the tests
	s.originalAppleJWKSURL = AppleJWKSURL // Store original from auth_helper
	AppleJWKSURL = s.mockAppleJWKSServer.URL // Point to mock server
}

// TearDownSuite runs once after all tests in the suite.
func (s *IntegrationTestSuite) TearDownSuite() {
	// Close mock Apple JWKS server
	if s.mockAppleJWKSServer != nil {
		s.mockAppleJWKSServer.Close()
	}
	// Restore original AppleJWKSURL
	AppleJWKSURL = s.originalAppleJWKSURL


	sqlDB, err := s.DB.DB()
	s.Require().NoError(err)
	err = sqlDB.Close()
	s.Require().NoError(err)
	s.Logger.Sync()
}

// TestIntegrationTestSuite runs the entire suite.
func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

// Example placeholder test (will be replaced by actual OAuth tests)
func (s *IntegrationTestSuite) TestPlaceholder() {
	s.Run("ExampleSubTest", func() {
		// Example: Test a non-existent auth route to see if router is alive
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/non-existent-dummy-route", nil)
		w := httptest.NewRecorder()
		s.Router.ServeHTTP(w, req)
		// For a non-existent route, Gin usually returns 404
		s.Equal(http.StatusNotFound, w.Code) // Example assertion
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Success_NewUser() {
	s.Run("Should register a new user via Google OAuth and return tokens", func() {
		// 1. Mock Google Token Endpoint
		mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Require().NoError(r.ParseForm())
			s.Equal("mock_auth_code", r.FormValue("code"))
			s.Equal(s.Cfg.GoogleClientID, r.FormValue("client_id"))
			// Note: Real Google OAuth might use Basic Auth for client_secret.
			// Here we assume it's in form value for simplicity or check if it matches your actual flow.
			// s.Equal(s.Cfg.GoogleClientSecret, r.FormValue("client_secret")) 
			s.Equal(s.Cfg.GoogleRedirectURI, r.FormValue("redirect_uri"))
			s.Equal("authorization_code", r.FormValue("grant_type"))

			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "access_token": "mock_google_access_token",
                "token_type": "Bearer",
                "refresh_token": "mock_google_refresh_token",
                "expires_in": 3600,
                "id_token": "mock_google_id_token"
            }`))
			s.Require().NoError(err)
		}))
		defer mockTokenServer.Close()

		// 2. Mock Google User Info Endpoint
		mockUserInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("Bearer mock_google_access_token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "sub": "mock_google_user_id_new",
                "email": "new_google_user@example.com",
                "email_verified": true,
                "given_name": "GoogleNew",
                "family_name": "User",
                "picture": "http://example.com/new_picture.jpg"
            }`))
			s.Require().NoError(err)
		}))
		defer mockUserInfoServer.Close()

		// 3. Configure oauthService for Mocks
		originalGoogleUserInfoURL := GoogleUserInfoURL // from auth_helper.go (now a var)
		GoogleUserInfoURL = mockUserInfoServer.URL
		defer func() { GoogleUserInfoURL = originalGoogleUserInfoURL }()

		// Modify the global google.Endpoint for the duration of this test
		// This influences the oauth2.Config used by getGoogleOAuthConfig
		originalGoogleEndpointTokenURL := google.Endpoint.TokenURL
		google.Endpoint.TokenURL = mockTokenServer.URL
		defer func() { google.Endpoint.TokenURL = originalGoogleEndpointTokenURL }()

		// 4. Set State Cookie
		testState := "state_for_new_user_test"
		// Create a request to the callback URL (the one handled by s.Router)
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+testState, nil)
		s.Require().NoError(err)
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: testState, Path: "/"})
		
		w := httptest.NewRecorder()

		// 5. Perform Request
		s.Router.ServeHTTP(w, req)

		// 6. Assertions
		s.Equal(http.StatusOK, w.Code, "HTTP status code should be OK")

		var responseBody struct {
			Message string            `json:"message"`
			User    user.UserResponse `json:"user"`
			Token   shared.TokenResponse `json:"token"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &responseBody)
		s.Require().NoError(err, "Failed to unmarshal response body: "+w.Body.String())

		s.Contains(responseBody.Message, "Google login successful", "Response message mismatch")
		s.Require().NotNil(responseBody.User.Email, "User email should not be nil")
		s.Equal("new_google_user@example.com", *responseBody.User.Email, "User email mismatch")
		s.Require().NotNil(responseBody.User.FirstName, "User FirstName should not be nil")
		s.Equal("GoogleNew", *responseBody.User.FirstName, "User first name mismatch")
		s.Require().NotNil(responseBody.User.LastName, "User LastName should not be nil")
		s.Equal("User", *responseBody.User.LastName, "User last name mismatch")
		s.Require().NotNil(responseBody.User.ProfilePictureURL, "User ProfilePictureURL should not be nil")
		s.Equal("http://example.com/new_picture.jpg", *responseBody.User.ProfilePictureURL, "User picture URL mismatch")
		s.NotEmpty(responseBody.Token.AccessToken, "Access token should not be empty")
		s.Equal("Bearer", responseBody.Token.TokenType, "Token type should be Bearer")

		// Verify database
		var dbUser user.User // GORM user model from 'seattle_info_backend/internal/user'
		result := s.DB.Where("email = ?", "new_google_user@example.com").First(&dbUser)
		s.Require().NoError(result.Error, "User should be created in the database")
		s.Require().NotNil(dbUser.ProviderID, "ProviderID should not be nil in DB")
		s.Equal("mock_google_user_id_new", *dbUser.ProviderID, "ProviderID mismatch in DB")
		s.Equal(string(ProviderGoogle), dbUser.AuthProvider, "AuthProvider mismatch in DB")
		s.True(dbUser.IsEmailVerified, "Email should be verified in DB")

		// Verify state cookie was cleared from the response
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestAppleCallback_Success_NewUser() {
	s.Run("Should register a new user via Apple OAuth and return tokens", func() {
		// 1. Generate Test Data
		state := "apple_test_state_success_new_user"
		nonce := "apple_test_nonce_success_new_user"
		email := "newappleuser@example.com"
		subject := "apple_user_subject_123_new"
		issuedAt := time.Now().Add(-time.Minute)      // Token issued 1 minute ago
		expiresAt := time.Now().Add(time.Hour * 1) // Token expires in 1 hour

		idToken, err := s.generateDummyAppleIDToken(
			s.Cfg.AppleClientID, // audience
			subject,
			email,
			nonce,
			issuedAt,
			expiresAt,
		)
		s.Require().NoError(err, "Failed to generate dummy Apple ID token")

		// 2. Prepare POST Form Data & Request
		formData := url.Values{}
		formData.Set("code", "mock_apple_auth_code_new_user") // Can be dummy
		formData.Set("id_token", idToken)
		formData.Set("state", state)

		// Simulate user providing name on first login
		userFormData := map[string]map[string]string{"name": {"firstName": "AppleNew", "lastName": "User"}}
		userJSONBytes, err := json.Marshal(userFormData)
		s.Require().NoError(err, "Failed to marshal user form data for Apple")
		formData.Set("user", string(userJSONBytes))
		
		reqBody := strings.NewReader(formData.Encode())
		req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/apple/callback", reqBody)
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// 3. Set Cookies
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: state, Path: "/"})
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthNonceCookieName, Value: nonce, Path: "/"})
		
		w := httptest.NewRecorder()

		// 4. Perform Request
		s.Router.ServeHTTP(w, req)

		// 5. Assertions
		s.Equal(http.StatusOK, w.Code, "HTTP status code should be OK")

		var responseBody struct {
			Message string            `json:"message"`
			User    user.UserResponse `json:"user"`
			Token   shared.TokenResponse `json:"token"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &responseBody)
		s.Require().NoError(err, "Failed to unmarshal response body: "+w.Body.String())

		s.Contains(responseBody.Message, "Apple Sign-In successful", "Response message mismatch")
		s.Require().NotNil(responseBody.User.Email, "User email should not be nil")
		s.Equal(email, *responseBody.User.Email, "User email mismatch")
		s.Require().NotNil(responseBody.User.FirstName, "User FirstName should not be nil")
		s.Equal("AppleNew", *responseBody.User.FirstName, "User first name mismatch")
		s.Require().NotNil(responseBody.User.LastName, "User LastName should not be nil")
		s.Equal("User", *responseBody.User.LastName, "User last name mismatch")
		s.NotEmpty(responseBody.Token.AccessToken, "Access token should not be empty")
		s.Equal("Bearer", responseBody.Token.TokenType, "Token type should be Bearer")

		// Verify database
		var dbUser user.User // GORM user model
		result := s.DB.Where("email = ?", email).First(&dbUser)
		s.Require().NoError(result.Error, "User should be created in the database")
		s.Require().NotNil(dbUser.ProviderID, "ProviderID should not be nil in DB")
		s.Equal(subject, *dbUser.ProviderID, "ProviderID (subject) mismatch in DB")
		s.Equal(string(ProviderApple), dbUser.AuthProvider, "AuthProvider mismatch in DB")
		s.True(dbUser.IsEmailVerified, "Email should be verified in DB (based on dummy token generation)")

		// Verify state cookie was cleared
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")

		// Verify nonce cookie was cleared
		var nonceCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthNonceCookieName {
                nonceCookie = c
                break
            }
        }
        s.Require().NotNil(nonceCookie, "Nonce cookie should be present in response to be cleared")
        s.LessOrEqual(nonceCookie.MaxAge, 0, "Nonce cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestAppleCallback_Error_GetAppleKeysFails() {
	s.Run("Should return an error if fetching Apple public keys (JWKS) fails", func() {
		// 1. Reset Apple Keys Cache
		ResetAppleKeysCacheForTest() // Use the function from auth_helper.go

		// 2. Override Mock JWKS Server to simulate failure
		// Stop the suite-level mock JWKS server first to avoid port conflicts if any
		if s.mockAppleJWKSServer != nil {
			s.mockAppleJWKSServer.Close()
		}

		mockErrorJWKSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "simulated JWKS fetch error", http.StatusInternalServerError)
		}))
		defer mockErrorJWKSServer.Close()

		originalSuiteAppleJWKSURL := AppleJWKSURL // This is auth.AppleJWKSURL
		AppleJWKSURL = mockErrorJWKSServer.URL
		
		// Defer restoration of the original suite-level mock JWKS server and URL
		defer func() {
			AppleJWKSURL = originalSuiteAppleJWKSURL
			// Restart the suite-level mock server if it was running
			// This is tricky because httptest.Server doesn't have a "Restart"
			// For simplicity in this test, we assume SetupSuite will run again for next suite or test if needed,
			// or that individual tests requiring the default mock JWKS server will set it up.
			// The s.originalAppleJWKSURL should be restored by TearDownSuite.
			// For now, just ensure the URL is restored. The suite-level server might need re-init in SetupSuite for subsequent tests.
			// A better way would be to not stop s.mockAppleJWKSServer but ensure auth.AppleJWKSURL points here.
			// Let's re-evaluate: s.originalAppleJWKSURL is set in SetupSuite.
			// The SetupSuite's defer s.mockAppleJWKSServer.Close() will run at the end of all tests.
			// We need to restore auth.AppleJWKSURL to s.mockAppleJWKSServer.URL if other tests need it.
			// The current s.originalAppleJWKSURL in SetupSuite is the *real* Apple URL.
			// So, after this test, AppleJWKSURL should be restored to s.mockAppleJWKSServer.URL (the one from SetupSuite).
			// This defer func() { AppleJWKSURL = originalSuiteAppleJWKSURL } is correct for this test's scope.
		}()


		// 3. Generate Test Data
		state := "apple_test_state_jwks_fail"
		nonce := "apple_test_nonce_jwks_fail"
		email := "applejwksfail@example.com"
		subject := "apple_user_subject_jwks_fail"
		issuedAt := time.Now().Add(-time.Minute)
		expiresAt := time.Now().Add(time.Hour * 1)

		// ID token can be validly signed with s.testApplePrivKey, as the failure is before signature check
		idToken, err := s.generateDummyAppleIDToken(
			s.Cfg.AppleClientID,
			subject,
			email,
			nonce,
			issuedAt,
			expiresAt,
		)
		s.Require().NoError(err, "Failed to generate dummy Apple ID token for JWKS fail test")

		// 4. Prepare POST Form Data & Request
		formData := url.Values{}
		formData.Set("id_token", idToken)
		formData.Set("state", state)
		formData.Set("code", "mock_apple_auth_code_jwks_fail")

		reqBody := strings.NewReader(formData.Encode())
		req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/apple/callback", reqBody)
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// 5. Set Cookies
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: state, Path: "/"})
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthNonceCookieName, Value: nonce, Path: "/"})
		
		w := httptest.NewRecorder()

		// 6. Perform Request
		s.Router.ServeHTTP(w, req)

		// 7. Assertions
		// Expecting common.ErrUnauthorized from verifyAppleIDToken because getApplePublicKeys fails.
		s.Equal(http.StatusUnauthorized, w.Code, "HTTP status code should be Unauthorized for JWKS fetch failure")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("UNAUTHORIZED", errorResponse.Code, "Error code should be UNAUTHORIZED")
		s.Contains(errorResponse.Message, "Invalid Apple ID token", "Error message should indicate invalid token")
		s.Contains(errorResponse.Message, "could not get apple public keys", "Error message detail for JWKS failure")

		// Verify state cookie was cleared
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")

		// Verify nonce cookie was cleared
		var nonceCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthNonceCookieName {
                nonceCookie = c
                break
            }
        }
        s.Require().NotNil(nonceCookie, "Nonce cookie should be present in response to be cleared")
        s.LessOrEqual(nonceCookie.MaxAge, 0, "Nonce cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestAppleCallback_Error_InvalidTokenSignature() {
	s.Run("Should return an error if Apple ID token signature is invalid", func() {
		// 1. Generate "Wrong" Key Pair (different from the one whose public key is in JWKS)
		wrongPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		s.Require().NoError(err, "Failed to generate 'wrong' ECDSA private key for Apple test")

		// 2. Generate Test Data
		state := "apple_test_state_invalid_sig"
		nonce := "apple_test_nonce_invalid_sig"
		email := "appleinvalid_sig@example.com"
		subject := "apple_user_subject_invalid_sig"
		issuedAt := time.Now().Add(-time.Minute)
		expiresAt := time.Now().Add(time.Hour * 1)

		// 3. Generate ID Token signed with the "wrong" private key
		// but using the KID of the *correct* public key (s.testApplePubKeyJWK.KeyID)
		// so that the mock JWKS server returns the correct public key for verification attempt.
		wrongSignerOpts := jose.SignerOptions{}.WithType("JWT").WithHeader("kid", s.testApplePubKeyJWK.KeyID) // Use correct KID
		wrongSigner, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: wrongPrivKey}, &wrongSignerOpts)
		s.Require().NoError(err, "Failed to create JWT signer with 'wrong' key")

		claims := josejwt.Claims{
			Issuer:   appleIssuer,
			Audience: josejwt.Audience{s.Cfg.AppleClientID},
			Subject:  subject,
			Expiry:   josejwt.NewNumericDate(expiresAt),
			IssuedAt: josejwt.NewNumericDate(issuedAt),
		}
		customClaims := struct {
			Email         string `json:"email,omitempty"`
			EmailVerified string `json:"email_verified,omitempty"`
			Nonce         string `json:"nonce,omitempty"`
		}{
			Email:         email,
			EmailVerified: "true",
			Nonce:         nonce,
		}
		idTokenSignedWithWrongKey, err := josejwt.Signed(wrongSigner).Claims(claims).Claims(customClaims).CompactSerialize()
		s.Require().NoError(err, "Failed to sign Apple ID token with 'wrong' key")


		// 4. Prepare POST Form Data & Request
		formData := url.Values{}
		formData.Set("id_token", idTokenSignedWithWrongKey)
		formData.Set("state", state)
		formData.Set("code", "mock_apple_auth_code_invalid_sig")

		reqBody := strings.NewReader(formData.Encode())
		req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/apple/callback", reqBody)
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// 5. Set Cookies
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: state, Path: "/"})
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthNonceCookieName, Value: nonce, Path: "/"})
		
		w := httptest.NewRecorder()

		// 6. Perform Request
		s.Router.ServeHTTP(w, req)

		// 7. Assertions
		// Expecting common.ErrUnauthorized from verifyAppleIDToken due to signature validation failure.
		s.Equal(http.StatusUnauthorized, w.Code, "HTTP status code should be Unauthorized for invalid signature")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("UNAUTHORIZED", errorResponse.Code, "Error code should be UNAUTHORIZED")
		// The exact message might be "failed to verify apple id_token claims signature" or similar.
		s.Contains(errorResponse.Message, "Invalid Apple ID token", "Error message should indicate invalid token")
		s.Contains(errorResponse.Message, "failed to verify", "Error message detail for signature failure")


		// Verify state cookie was cleared
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")

		// Verify nonce cookie was cleared
		var nonceCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthNonceCookieName {
                nonceCookie = c
                break
            }
        }
        s.Require().NotNil(nonceCookie, "Nonce cookie should be present in response to be cleared")
        s.LessOrEqual(nonceCookie.MaxAge, 0, "Nonce cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestAppleCallback_Error_NonceMismatch() {
	s.Run("Should return an error if Apple OAuth nonce in cookie and ID token do not match", func() {
		// 1. Generate Test Data
		state := "apple_test_state_nonce_mismatch"
		cookieNonce := "apple_cookie_nonce_value"
		tokenNonce := "apple_token_nonce_that_is_different" // Mismatched nonce in ID token
		email := "applenoncemismatch@example.com"
		subject := "apple_user_subject_nonce_mismatch"
		issuedAt := time.Now().Add(-time.Minute)
		expiresAt := time.Now().Add(time.Hour * 1)

		// Generate ID token with tokenNonce
		idToken, err := s.generateDummyAppleIDToken(
			s.Cfg.AppleClientID,
			subject,
			email,
			tokenNonce, // Use the tokenNonce here
			issuedAt,
			expiresAt,
		)
		s.Require().NoError(err, "Failed to generate dummy Apple ID token for nonce mismatch test")

		// 2. Prepare POST Form Data & Request
		formData := url.Values{}
		formData.Set("id_token", idToken)
		formData.Set("state", state)
		formData.Set("code", "mock_apple_auth_code_nonce_mismatch")


		reqBody := strings.NewReader(formData.Encode())
		req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/apple/callback", reqBody)
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// 3. Set Cookies
		// Set the state and cookieNonce in the cookies
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: state, Path: "/"})
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthNonceCookieName, Value: cookieNonce, Path: "/"})
		
		w := httptest.NewRecorder()

		// 4. Perform Request
		s.Router.ServeHTTP(w, req)

		// 5. Assertions
		// Expecting common.ErrUnauthorized with details from verifyAppleIDToken
		// e.g., "Invalid Apple ID token: apple id_token nonce mismatch..."
		s.Equal(http.StatusUnauthorized, w.Code, "HTTP status code should be Unauthorized for nonce mismatch")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("UNAUTHORIZED", errorResponse.Code, "Error code should be UNAUTHORIZED")
		s.Contains(errorResponse.Message, "apple id_token nonce mismatch", "Error message should indicate nonce mismatch")

		// Verify state cookie was cleared
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")

		// Verify nonce cookie was cleared
		var nonceCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthNonceCookieName {
                nonceCookie = c
                break
            }
        }
        s.Require().NotNil(nonceCookie, "Nonce cookie should be present in response to be cleared")
        s.LessOrEqual(nonceCookie.MaxAge, 0, "Nonce cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestAppleCallback_Error_StateMismatch() {
	s.Run("Should return an error if Apple OAuth state in cookie and form do not match", func() {
		// 1. Generate Test Data
		originalState := "apple_original_state_for_mismatch_test"
		callbackState := "apple_callback_state_that_does_not_match" // Mismatched state
		nonce := "apple_test_nonce_for_state_mismatch"
		email := "apple_statemismatch@example.com"
		subject := "apple_user_subject_statemismatch"
		issuedAt := time.Now().Add(-time.Minute)
		expiresAt := time.Now().Add(time.Hour * 1)

		idToken, err := s.generateDummyAppleIDToken(
			s.Cfg.AppleClientID,
			subject,
			email,
			nonce, // Nonce in token should match cookie for nonce check to pass if state check failed first
			issuedAt,
			expiresAt,
		)
		s.Require().NoError(err, "Failed to generate dummy Apple ID token for state mismatch test")

		// 2. Prepare POST Form Data & Request
		formData := url.Values{}
		formData.Set("id_token", idToken)
		formData.Set("state", callbackState) // Use the mismatched state in the form post
		// "code" can be omitted or dummy, as state check should fail before code is used by Apple flow.
		formData.Set("code", "mock_apple_auth_code_state_mismatch")


		reqBody := strings.NewReader(formData.Encode())
		req, err := http.NewRequest(http.MethodPost, "/api/v1/auth/apple/callback", reqBody)
		s.Require().NoError(err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// 3. Set Cookies
		// Set the original_state in the cookie
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: originalState, Path: "/"})
		// Nonce cookie must also be present for the handler to proceed to state check.
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthNonceCookieName, Value: nonce, Path: "/"})
		
		w := httptest.NewRecorder()

		// 4. Perform Request
		s.Router.ServeHTTP(w, req)

		// 5. Assertions
		// Expecting common.ErrBadRequest with details "OAuth state mismatch. Possible CSRF attack."
		s.Equal(http.StatusBadRequest, w.Code, "HTTP status code should be Bad Request for state mismatch")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("BAD_REQUEST", errorResponse.Code, "Error code should be BAD_REQUEST")
		s.Contains(errorResponse.Message, "OAuth state mismatch", "Error message should indicate state mismatch")

		// Verify state cookie was cleared (or attempted to be cleared)
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")

		// Nonce cookie should also be cleared as part of the getOAuthCookie logic
		var nonceCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthNonceCookieName {
                nonceCookie = c
                break
            }
        }
        s.Require().NotNil(nonceCookie, "Nonce cookie should be present in response to be cleared")
        s.LessOrEqual(nonceCookie.MaxAge, 0, "Nonce cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Error_GetUserInfoFails() {
	s.Run("Should return an error if fetching Google user info fails", func() {
		// 1. Mock Google Token Endpoint (Successful)
		mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "access_token": "mock_google_access_token_for_userinfo_fail",
                "token_type": "Bearer",
                "expires_in": 3600
            }`))
			s.Require().NoError(err)
		}))
		defer mockTokenServer.Close()

		// 2. Mock Google User Info Endpoint (Failure)
		mockUserInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("Bearer mock_google_access_token_for_userinfo_fail", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusInternalServerError) // Simulate server error from Google
			_, err := w.Write([]byte(`{"error": "server_error", "error_description": "Internal Google error"}`))
			s.Require().NoError(err)
		}))
		defer mockUserInfoServer.Close()

		// 3. Configure oauthService for Mocks
		originalGoogleUserInfoURL := GoogleUserInfoURL // from auth_helper.go
		GoogleUserInfoURL = mockUserInfoServer.URL
		defer func() { GoogleUserInfoURL = originalGoogleUserInfoURL }()

		originalGoogleEndpointTokenURL := google.Endpoint.TokenURL
		google.Endpoint.TokenURL = mockTokenServer.URL
		defer func() { google.Endpoint.TokenURL = originalGoogleEndpointTokenURL }()

		// 4. Set State Cookie
		testState := "state_for_userinfo_fail_test"
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+testState, nil)
		s.Require().NoError(err)
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: testState, Path: "/"})
		
		w := httptest.NewRecorder()

		// 5. Perform Request
		s.Router.ServeHTTP(w, req)

		// 6. Assertions
		// The handler's s.OAuthSvc.HandleGoogleCallback is expected to return:
		// common.ErrServiceUnavailable.WithDetails("Could not fetch user info from Google.")
		// or if the status code from Google is directly translated:
		// common.ErrServiceUnavailable.WithDetails(fmt.Sprintf("Google returned status %d for user info.", userInfoResp.StatusCode))
		s.Equal(http.StatusServiceUnavailable, w.Code, "HTTP status code should be Service Unavailable")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("SERVICE_UNAVAILABLE", errorResponse.Code, "Error code mismatch")
		// The message might vary slightly depending on if the body is read or just status code is used.
		// The current implementation logs the body but returns a generic "Could not fetch user info from Google."
		// if the client.Get itself fails, or a specific message if the status code is not OK.
		// Let's check for the specific one related to status code not OK.
		s.Contains(errorResponse.Message, "Google returned status 500 for user info", "Error message mismatch")


		// Verify state cookie was cleared from the response even on error
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Error_ExchangeCodeFails() {
	s.Run("Should return an error if exchanging Google auth code for token fails", func() {
		// 1. Mock Google Token Endpoint to return an error
		mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest) // Example error from Google
			_, err := w.Write([]byte(`{"error": "invalid_grant", "error_description": "Bad Request"}`))
			s.Require().NoError(err)
		}))
		defer mockTokenServer.Close()

		// 2. Configure oauthService for Mock
		// Modify the global google.Endpoint.TokenURL for the duration of this test
		originalGoogleEndpointTokenURL := google.Endpoint.TokenURL
		google.Endpoint.TokenURL = mockTokenServer.URL
		defer func() { google.Endpoint.TokenURL = originalGoogleEndpointTokenURL }()
		
		// Note: We don't need to mock GoogleUserInfoURL as the flow should fail before that.

		// 3. Set State Cookie
		testState := "state_for_exchange_code_fail_test"
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+testState, nil)
		s.Require().NoError(err)
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: testState, Path: "/"})
		
		w := httptest.NewRecorder()

		// 4. Perform Request
		s.Router.ServeHTTP(w, req)

		// 5. Assertions
		// The handler's s.OAuthSvc.HandleGoogleCallback is expected to return:
		// common.ErrServiceUnavailable.WithDetails("Could not exchange Google auth code.")
		// which has StatusCode http.StatusServiceUnavailable
		s.Equal(http.StatusServiceUnavailable, w.Code, "HTTP status code should be Service Unavailable")

		var errorResponse common.APIError
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("SERVICE_UNAVAILABLE", errorResponse.Code, "Error code mismatch")
		s.Contains(errorResponse.Message, "Could not exchange Google auth code", "Error message mismatch")

		// Verify state cookie was cleared from the response even on error
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Error_StateMismatch() {
	s.Run("Should return an error if OAuth state in cookie and query param do not match", func() {
		// 1. Set State Cookie (Original State)
		originalState := "original_test_state_value"
		callbackState := "different_callback_state_value" // This state will be in the URL query

		// Create a request to the callback URL
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+callbackState, nil)
		s.Require().NoError(err)
		// Add the original state as a cookie
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: originalState, Path: "/"})
		
		w := httptest.NewRecorder()

		// 2. Perform Request (with different state in query)
		s.Router.ServeHTTP(w, req)

		// 3. Assertions
		// The handler should return common.ErrBadRequest.WithDetails("OAuth state mismatch. Possible CSRF attack.")
		// common.ErrBadRequest has StatusCode http.StatusBadRequest
		s.Equal(http.StatusBadRequest, w.Code, "HTTP status code should be Bad Request")

		var errorResponse common.APIError // Assuming common.APIError is the structure for error responses
		err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
		s.Require().NoError(err, "Failed to unmarshal error response body: "+w.Body.String())

		s.Equal("BAD_REQUEST", errorResponse.Code, "Error code mismatch") // Or whatever code is set by ErrBadRequest
		s.Contains(errorResponse.Message, "OAuth state mismatch", "Error message should indicate state mismatch")

		// Verify state cookie was cleared from the response even on error
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Success_NewUser() {
	s.Run("Should register a new user via Google OAuth and return tokens", func() {
		// 1. Mock Google Token Endpoint
		mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Require().NoError(r.ParseForm())
			s.Equal("mock_auth_code", r.FormValue("code"))
			s.Equal(s.Cfg.GoogleClientID, r.FormValue("client_id"))
			// Note: Real Google OAuth might use Basic Auth for client_secret.
			// Here we assume it's in form value for simplicity or check if it matches your actual flow.
			// s.Equal(s.Cfg.GoogleClientSecret, r.FormValue("client_secret")) 
			s.Equal(s.Cfg.GoogleRedirectURI, r.FormValue("redirect_uri"))
			s.Equal("authorization_code", r.FormValue("grant_type"))

			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "access_token": "mock_google_access_token",
                "token_type": "Bearer",
                "refresh_token": "mock_google_refresh_token",
                "expires_in": 3600,
                "id_token": "mock_google_id_token"
            }`))
			s.Require().NoError(err)
		}))
		defer mockTokenServer.Close()

		// 2. Mock Google User Info Endpoint
		mockUserInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("Bearer mock_google_access_token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "sub": "mock_google_user_id_new",
                "email": "new_google_user@example.com",
                "email_verified": true,
                "given_name": "GoogleNew",
                "family_name": "User",
                "picture": "http://example.com/new_picture.jpg"
            }`))
			s.Require().NoError(err)
		}))
		defer mockUserInfoServer.Close()

		// 3. Configure oauthService for Mocks
		originalGoogleUserInfoURL := GoogleUserInfoURL // from auth_helper.go (now a var)
		GoogleUserInfoURL = mockUserInfoServer.URL
		defer func() { GoogleUserInfoURL = originalGoogleUserInfoURL }()

		// Modify the global google.Endpoint for the duration of this test
		// This influences the oauth2.Config used by getGoogleOAuthConfig
		originalGoogleEndpointTokenURL := google.Endpoint.TokenURL
		google.Endpoint.TokenURL = mockTokenServer.URL
		defer func() { google.Endpoint.TokenURL = originalGoogleEndpointTokenURL }()

		// 4. Set State Cookie
		testState := "state_for_new_user_test"
		// Create a request to the callback URL (the one handled by s.Router)
		req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+testState, nil)
		s.Require().NoError(err)
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: testState, Path: "/"})
		
		w := httptest.NewRecorder()

		// 5. Perform Request
		s.Router.ServeHTTP(w, req)

		// 6. Assertions
		s.Equal(http.StatusOK, w.Code, "HTTP status code should be OK")

		var responseBody struct {
			Message string            `json:"message"`
			User    user.UserResponse `json:"user"`
			Token   shared.TokenResponse `json:"token"`
		}
		err = json.Unmarshal(w.Body.Bytes(), &responseBody)
		s.Require().NoError(err, "Failed to unmarshal response body: "+w.Body.String())

		s.Contains(responseBody.Message, "Google login successful", "Response message mismatch")
		s.Require().NotNil(responseBody.User.Email, "User email should not be nil")
		s.Equal("new_google_user@example.com", *responseBody.User.Email, "User email mismatch")
		s.Require().NotNil(responseBody.User.FirstName, "User FirstName should not be nil")
		s.Equal("GoogleNew", *responseBody.User.FirstName, "User first name mismatch")
		s.Require().NotNil(responseBody.User.LastName, "User LastName should not be nil")
		s.Equal("User", *responseBody.User.LastName, "User last name mismatch")
		s.Require().NotNil(responseBody.User.ProfilePictureURL, "User ProfilePictureURL should not be nil")
		s.Equal("http://example.com/new_picture.jpg", *responseBody.User.ProfilePictureURL, "User picture URL mismatch")
		s.NotEmpty(responseBody.Token.AccessToken, "Access token should not be empty")
		s.Equal("Bearer", responseBody.Token.TokenType, "Token type should be Bearer")

		// Verify database
		var dbUser user.User // GORM user model from 'seattle_info_backend/internal/user'
		result := s.DB.Where("email = ?", "new_google_user@example.com").First(&dbUser)
		s.Require().NoError(result.Error, "User should be created in the database")
		s.Require().NotNil(dbUser.ProviderID, "ProviderID should not be nil in DB")
		s.Equal("mock_google_user_id_new", *dbUser.ProviderID, "ProviderID mismatch in DB")
		s.Equal(string(ProviderGoogle), dbUser.AuthProvider, "AuthProvider mismatch in DB")
		s.True(dbUser.IsEmailVerified, "Email should be verified in DB")

		// Verify state cookie was cleared from the response
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}

func (s *IntegrationTestSuite) TestGoogleCallback_Success_NewUser() {
	s.Run("Should register a new user via Google OAuth and return tokens", func() {
		// 1. Mock Google Token Endpoint
		mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Require().NoError(r.ParseForm())
			s.Equal("mock_auth_code", r.FormValue("code"))
			s.Equal(s.Cfg.GoogleClientID, r.FormValue("client_id"))
			// Client secret check might be tricky if it's not directly in form but basic auth header
			// For this test, we assume it's okay or client_secret is passed in body.
			// s.Equal(s.Cfg.GoogleClientSecret, r.FormValue("client_secret")) 
			s.Equal(s.Cfg.GoogleRedirectURI, r.FormValue("redirect_uri"))
			s.Equal("authorization_code", r.FormValue("grant_type"))

			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "access_token": "mock_google_access_token",
                "token_type": "Bearer",
                "refresh_token": "mock_google_refresh_token",
                "expires_in": 3600,
                "id_token": "mock_google_id_token"
            }`))
			s.Require().NoError(err)
		}))
		defer mockTokenServer.Close()

		// 2. Mock Google User Info Endpoint
		mockUserInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("Bearer mock_google_access_token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{
                "sub": "mock_google_user_id_123",
                "email": "testuser@example.com",
                "email_verified": true,
                "given_name": "Test",
                "family_name": "User",
                "picture": "http://example.com/picture.jpg"
            }`))
			s.Require().NoError(err)
		}))
		defer mockUserInfoServer.Close()

		// 3. Configure oauthService for Mocks
		// Directly modify the Google OAuth config used by the service
		// Get the original config, modify its TokenURL, and then re-set it or ensure auth_helper uses it.
		// For this test, we'll modify the global GoogleUserInfoURL from auth_helper
		// and assume getGoogleOAuthConfig uses the Cfg for redirect URI and client creds.
		
		originalGoogleUserInfoURL := GoogleUserInfoURL // from auth_helper.go
		GoogleUserInfoURL = mockUserInfoServer.URL
		defer func() { GoogleUserInfoURL = originalGoogleUserInfoURL }()

		// The getGoogleOAuthConfig helper needs to use our mock token server URL.
		// We achieve this by temporarily modifying the global google.Endpoint for the test duration
		// This is a bit hacky. A better way would be to inject the *oauth2.Config into oauthService
		// or make getGoogleOAuthConfig return a config that can be modified.
		// For now, let's assume we can influence it through s.Cfg if possible, or use a more direct approach.
		// The HandleGoogleCallback uses `getGoogleOAuthConfig(s.cfg)`.
		// We need that config's TokenURL to be our mock.
		// The `getGoogleOAuthConfig` function internally sets `Endpoint: google.Endpoint`.
		// The most straightforward way without refactoring `getGoogleOAuthConfig` deeply is
		// to ensure that the `oauth2.Config` instance used by `s.OAuthSvc.(*oauthService).HandleGoogleCallback`
		// has its `Endpoint.TokenURL` set.
		// This is tricky because `getGoogleOAuthConfig` is called inside `HandleGoogleCallback`.
		// A simpler approach for testing *this handler* is to ensure the config values are set.
		// The `oauthService.HandleGoogleCallback` uses `getGoogleOAuthConfig(s.Cfg)`.
		// Let's assume `getGoogleOAuthConfig` is:
		// func getGoogleOAuthConfig(cfg *config.Config) *oauth2.Config {
		//   return &oauth2.Config{ ..., Endpoint: google.Endpoint, ...}
		// }
		// We will rely on the fact that `google.Endpoint.TokenURL` is a public var we can change.
		// This is a common but somewhat fragile way to test external endpoints.
		originalGoogleEndpointTokenURL := google.Endpoint.TokenURL
		google.Endpoint.TokenURL = mockTokenServer.URL
		defer func() { google.Endpoint.TokenURL = originalGoogleEndpointTokenURL }()


		// 4. Set State Cookie
		state := "test_state_12345"
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=mock_auth_code&state="+state, nil)
		req.AddCookie(&http.Cookie{Name: s.Cfg.OAuthStateCookieName, Value: state, Path: "/"})
		
		w := httptest.NewRecorder()

		// 5. Perform Request
		s.Router.ServeHTTP(w, req)

		// 6. Assertions
		s.Equal(http.StatusOK, w.Code, "HTTP status code should be OK")

		var responseBody struct {
			Message string            `json:"message"`
			User    user.UserResponse `json:"user"` // Assuming user.UserResponse is the structure
			Token   shared.TokenResponse `json:"token"`  // Assuming shared.TokenResponse
		}
		err := json.Unmarshal(w.Body.Bytes(), &responseBody)
		s.Require().NoError(err, "Failed to unmarshal response body")

		s.Contains(responseBody.Message, "Google login successful", "Response message mismatch")
		s.Equal("testuser@example.com", *responseBody.User.Email, "User email mismatch")
		s.Equal("Test", *responseBody.User.FirstName, "User first name mismatch")
		s.Equal("User", *responseBody.User.LastName, "User last name mismatch")
		s.Equal("http://example.com/picture.jpg", *responseBody.User.ProfilePictureURL, "User picture URL mismatch")
		s.NotEmpty(responseBody.Token.AccessToken, "Access token should not be empty")
		s.Equal("Bearer", responseBody.Token.TokenType, "Token type should be Bearer")

		// Verify database
		var dbUser user.User // GORM user model
		result := s.DB.Where("email = ?", "testuser@example.com").First(&dbUser)
		s.Require().NoError(result.Error, "User should be created in the database")
		s.Equal("mock_google_user_id_123", *dbUser.ProviderID, "ProviderID mismatch in DB")
		s.Equal(string(ProviderGoogle), dbUser.AuthProvider, "AuthProvider mismatch in DB")
		s.True(dbUser.IsEmailVerified, "Email should be verified in DB")

		// Verify state cookie was cleared
		foundStateCookie := false
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == s.Cfg.OAuthStateCookieName {
				foundStateCookie = true
				s.LessOrEqual(cookie.MaxAge, 0, "State cookie should be cleared (MaxAge <= 0)")
				break
			}
		}
		// If cookie is not found, it means it was cleared (MaxAge < 0 makes browser discard it).
		// Depending on exact SetCookie behavior for deletion, not finding it might be the primary check.
		// For this test, if it's found, its MaxAge must be <=0. If not found, it's also good.
		// The getOAuthCookie helper sets MaxAge to -1.
		// So, if found, it must have MaxAge <=0. If not found, it's fine too.
		// Let's refine: cookie *should* be present with MaxAge <=0 due to how SetCookie for deletion works.
		var stateCookie *http.Cookie
        for _, c := range w.Result().Cookies() {
            if c.Name == s.Cfg.OAuthStateCookieName {
                stateCookie = c
                break
            }
        }
        s.Require().NotNil(stateCookie, "State cookie should be present in response to be cleared")
        s.LessOrEqual(stateCookie.MaxAge, 0, "State cookie MaxAge should be <= 0, indicating it's cleared")
	})
}
