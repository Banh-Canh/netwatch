package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"

	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// verifier is a global variable to store the OIDC token verifier.
var verifier *oidc.IDTokenVerifier

// InitOIDCVerifier sets up the OIDC provider and verifier once at application startup.
func InitOIDCVerifier(issuerURL, clientID string) error {
	// Discover the OIDC provider's configuration.
	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		logger.Logger.Error("Failed to get OIDC provider", "error", err)
		return err
	}
	// Create a verifier that checks the audience (ClientID) of the token.
	verifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	logger.Logger.Info("Successfully initialized OIDC verifier.")
	return nil
}

// AuthMiddleware is a Gin middleware that handles two types of authentication:
// 1. OIDC Bearer tokens for authenticated users.
// 2. A static API key for programmatic access. I plan to make a CLI.
func AuthMiddleware(staticAPIToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		// If no Authorization header is present, the request continues without authentication.
		if authHeader == "" {
			logger.Logger.Debug("Authorization header not found. Skipping auth.")
			c.Next()
			return
		}
		// Split the header to get the authentication type and token.
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 {
			logger.Logger.Warn("Invalid Authorization header format", "header", authHeader)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header format must be {prefix} {token}"})
			return
		}
		authType := parts[0]
		tokenString := parts[1]
		switch authType {
		case "Bearer":
			// Verify the OIDC token. This checks signature, expiry, and other claims.
			idToken, err := verifier.Verify(c.Request.Context(), tokenString)
			if err != nil {
				logger.Logger.Warn("Invalid OIDC token", "error", err)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid OIDC token"})
				return
			}
			// Set the token and user claims in the Gin context for later use by handlers.
			c.Set("id_token", tokenString)
			var claims struct {
				Email string `json:"email"`
			}
			if idToken.Claims(&claims) == nil {
				c.Set("user", claims.Email)
				logger.Logger.Info("Authenticated with OIDC token", "user", claims.Email, "email", claims.Email)
			}
		case "ApiKey":
			// Use subtle.ConstantTimeCompare to prevent timing attacks when comparing API keys. Forgot the source.
			if subtle.ConstantTimeCompare([]byte(tokenString), []byte(staticAPIToken)) != 1 {
				logger.Logger.Warn("Invalid API key provided")
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
				return
			}
			// Set a generic user for API key-based requests.
			c.Set("user", "api-key-user")
			logger.Logger.Info("Authenticated with static API key")
		default:
			logger.Logger.Warn("Unsupported authorization type", "type", authType)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unsupported authorization type"})
			return
		}
		// Continue to the next middleware or handler.
		c.Next()
	}
}
