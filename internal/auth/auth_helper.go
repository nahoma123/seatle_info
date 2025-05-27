package auth

import (
	"crypto/ecdsa"
	"crypto/rsa" // ADDED for RSA public key type
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"seattle_info_backend/internal/config"
	"seattle_info_backend/internal/platform/crypto"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	// gocrypto "gopkg.in/square/go-jose.v2/cryptosigner" // REMOVE or ensure correct usage
	"gopkg.in/square/go-jose.v2"
	josejwt "gopkg.in/square/go-jose.v2/jwt"
)

var (
	// googleAuthURL remains const as it's used by oauth2.google.Endpoint
	googleAuthURL  = "https://accounts.google.com/o/oauth2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	// GoogleUserInfoURL is made a variable for testing
	GoogleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

const (
	appleAuthURL  = "https://appleid.apple.com/auth/authorize"
	appleTokenURL = "https://appleid.apple.com/auth/token"
	// appleJWKSURL made a variable for testing
	appleIssuer = "https://appleid.apple.com"
)

var (
	// AppleJWKSURL is made a variable for testing
	AppleJWKSURL = "https://appleid.apple.com/auth/keys"
)

// setOAuthCookie sets a secure cookie for state or nonce.
func setOAuthCookie(c *gin.Context, cfg *config.Config, name, value string) {
	maxAge := cfg.OAuthCookieMaxAgeMinutes * 60
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   cfg.OAuthCookieDomain,
		MaxAge:   maxAge,
		Secure:   cfg.OAuthCookieSecure,
		HttpOnly: cfg.OAuthCookieHTTPOnly,
		SameSite: parseSameSite(cfg.OAuthCookieSameSite),
	})
}

// getOAuthCookie retrieves and deletes an OAuth cookie.
func getOAuthCookie(c *gin.Context, cfg *config.Config, name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", fmt.Errorf("%s cookie not found: %w", name, err)
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Domain:   cfg.OAuthCookieDomain,
		MaxAge:   -1,
		Secure:   cfg.OAuthCookieSecure,
		HttpOnly: cfg.OAuthCookieHTTPOnly,
		SameSite: parseSameSite(cfg.OAuthCookieSameSite),
	})
	return cookie.Value, nil
}

func parseSameSite(s string) http.SameSite {
	switch s {
	case "Lax":
		return http.SameSiteLaxMode
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func generateAndSetOAuthState(c *gin.Context, cfg *config.Config) (string, error) {
	state, err := crypto.GenerateSecureRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	setOAuthCookie(c, cfg, cfg.OAuthStateCookieName, state)
	return state, nil
}

func generateAndSetOAuthNonce(c *gin.Context, cfg *config.Config) (string, error) {
	nonce, err := crypto.GenerateSecureRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	setOAuthCookie(c, cfg, cfg.OAuthNonceCookieName, nonce)
	return nonce, nil
}

func getGoogleOAuthConfig(cfg *config.Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURI,
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     google.Endpoint,
	}
}

func loadApplePrivateKey(path string) (*ecdsa.PrivateKey, error) {
	keyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read apple private key file '%s': %w", path, err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		pkcs1Key, errPkcs1 := x509.ParseECPrivateKey(block.Bytes)
		if errPkcs1 != nil {
			return nil, fmt.Errorf("failed to parse apple private key (tried PKCS8 and PKCS1): pkcs8_err=%v, pkcs1_err=%v", err, errPkcs1)
		}
		return pkcs1Key, nil
	}

	ecdsaPrivateKey, ok := privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not of type ECDSA")
	}
	return ecdsaPrivateKey, nil
}

func generateAppleClientSecret(cfg *config.Config) (string, error) {
	privateKey, err := loadApplePrivateKey(cfg.ApplePrivateKeyPath)
	if err != nil {
		return "", fmt.Errorf("could not load apple private key: %w", err)
	}

	claims := jwt.MapClaims{
		"iss": cfg.AppleTeamID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Minute * 5).Unix(),
		"aud": appleIssuer,
		"sub": cfg.AppleClientID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = cfg.AppleKeyID

	clientSecret, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign apple client secret: %w", err)
	}
	return clientSecret, nil
}

var appleKeysCache *jose.JSONWebKeySet
var appleKeysCacheExpiry time.Time
var appleKeysCacheLock = make(chan struct{}, 1)

func getApplePublicKeys() (*jose.JSONWebKeySet, error) {
	appleKeysCacheLock <- struct{}{}
	defer func() { <-appleKeysCacheLock }()

	if appleKeysCache != nil && time.Now().Before(appleKeysCacheExpiry) {
		return appleKeysCache, nil
	}

	resp, err := http.Get(appleJWKSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch apple public keys from %s: %w", appleJWKSURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch apple public keys: status %s, body: %s", resp.Status, string(bodyBytes))
	}

	var jwks jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode apple public keys JSON: %w", err)
	}

	appleKeysCache = &jwks
	appleKeysCacheExpiry = time.Now().Add(24 * time.Hour)
	return appleKeysCache, nil
}

type AppleIDTokenClaims struct {
	josejwt.Claims
	Email           string `json:"email,omitempty"`
	EmailVerified   string `json:"email_verified,omitempty"`
	IsPrivateEmail  string `json:"is_private_email,omitempty"`
	AuthTime        int64  `json:"auth_time,omitempty"`
	Nonce           string `json:"nonce,omitempty"`
	NonceSupported  bool   `json:"nonce_supported,omitempty"`
	RealUserStatus  int    `json:"real_user_status,omitempty"`
	TransferSub     string `json:"transfer_sub,omitempty"`
	CHash           string `json:"c_hash,omitempty"`
	AccessTokenHash string `json:"at_hash,omitempty"`
}

// verifyAppleIDToken validates the Apple ID token using go-jose.
func verifyAppleIDToken(idTokenStr string, clientID string, expectedNonce string) (*AppleIDTokenClaims, error) {
	parsedToken, err := josejwt.ParseSigned(idTokenStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse apple id_token: %w", err)
	}

	jwks, err := getApplePublicKeys()
	if err != nil {
		return nil, fmt.Errorf("could not get apple public keys for verification: %w", err)
	}

	var verificationKey interface{}
	var kidUsed string // To log which kid was used or not found

	// Find the key by kid from token header
	// Apple ID tokens should have a "kid" in their header.
	for _, header := range parsedToken.Headers {
		if header.KeyID != "" {
			kidUsed = header.KeyID
			keys := jwks.Key(header.KeyID)
			if len(keys) > 0 {
				// The Key field of jose.JSONWebKey is an interface{}
				// It should be *ecdsa.PublicKey for Apple's ES256 tokens
				// or *rsa.PublicKey if Apple ever used RSA (they don't for ID tokens).
				actualKey := keys[0].Key
				switch actualKey.(type) {
				case *ecdsa.PublicKey, *rsa.PublicKey:
					verificationKey = actualKey
				default:
					return nil, fmt.Errorf("unexpected key type in JWKS for kid %s: %T", header.KeyID, actualKey)
				}
				break // Found the key
			}
		}
	}

	if verificationKey == nil {
		if kidUsed != "" {
			return nil, fmt.Errorf("apple id_token signing key with kid '%s' not found in JWKS", kidUsed)
		}
		return nil, errors.New("apple id_token 'kid' header missing or signing key not found in JWKS")
	}

	claims := &AppleIDTokenClaims{}
	if err := parsedToken.Claims(verificationKey, claims); err != nil {
		return nil, fmt.Errorf("failed to verify apple id_token claims signature or structure: %w", err)
	}

	expected := josejwt.Expected{
		Issuer:   appleIssuer,
		Audience: josejwt.Audience{clientID},
		Time:     time.Now(),
	}
	if err := claims.Validate(expected); err != nil {
		return nil, fmt.Errorf("apple id_token standard claims validation failed: %w", err)
	}

	if claims.Nonce != expectedNonce {
		return nil, fmt.Errorf("apple id_token nonce mismatch: expected '%s', got '%s'", expectedNonce, claims.Nonce)
	}

	return claims, nil
}

type AppleUserForm struct {
	Name struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"name"`
	Email string `json:"email"`
}

// ResetAppleKeysCacheForTest clears the cached Apple public keys. For testing purposes only.
func ResetAppleKeysCacheForTest() {
	appleKeysCacheLock <- struct{}{}
	defer func() { <-appleKeysCacheLock }()
	appleKeysCache = nil
	appleKeysCacheExpiry = time.Time{}
}
