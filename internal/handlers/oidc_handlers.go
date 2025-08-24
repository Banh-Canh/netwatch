package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// This file contains all handlers and configuration related to the OIDC authentication flow.

var (
	oidcVerifier *oidc.IDTokenVerifier
	oidcConfig   *oauth2.Config // This is a base config without a RedirectURL
	sessionTTL   int
)

type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	SessionTTL   int
}

// InitOIDC initializes the OIDC provider and configuration for the handlers package.
func InitOIDC(cfg OIDCConfig) error {
	provider, err := oidc.NewProvider(context.Background(), cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("failed to get OIDC provider: %w", err)
	}
	oidcVerifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	// Initialize the base config. The RedirectURL will be generated.
	oidcConfig = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
	sessionTTL = cfg.SessionTTL
	return nil
}

// HandleMainPage renders the main layout template with session information.
func HandleMainPage(version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, _ := sessionStore.Get(c.Request, "auth-session")
		idToken, _ := session.Values["id_token"].(string)
		user, _ := session.Values["user"].(string)

		var sessionExpiresAt int64
		if exp, ok := session.Values["expires_at"].(int64); ok {
			sessionExpiresAt = exp
		} else {
			sessionExpiresAt = time.Now().Add(time.Second * time.Duration(sessionTTL)).Unix()
		}

		// Pass the version to the template here.
		c.HTML(http.StatusOK, "layout.html", gin.H{
			"IDToken":          idToken,
			"User":             user,
			"UserIP":           c.ClientIP(),
			"SessionExpiresAt": sessionExpiresAt,
			"Version":          version, // Add the version to the data map
		})
	}
}

// HandleLogin redirects the user to the OIDC provider for authentication.
func HandleLogin(c *gin.Context) {
	session, err := sessionStore.Get(c.Request, "auth-session")
	if err != nil {
		http.Error(c.Writer, "Failed to get session", http.StatusInternalServerError)
		return
	}

	stateBytes := make([]byte, 16)
	rand.Read(stateBytes) //nolint:all
	state := base64.URLEncoding.EncodeToString(stateBytes)
	session.Values["state"] = state
	session.Save(c.Request, c.Writer) //nolint:all

	authCfg := *oidcConfig
	authCfg.RedirectURL = GetBaseURL(c.Request) + "/auth/callback"

	// Now generate the URL from the temporary config. No extra options needed.
	authCodeURL := authCfg.AuthCodeURL(state)

	http.Redirect(c.Writer, c.Request, authCodeURL, http.StatusFound)
}

// HandleCallback receives the response from the OIDC provider.
func HandleCallback(c *gin.Context) {
	session, err := sessionStore.Get(c.Request, "auth-session")
	if err != nil {
		http.Error(c.Writer, "Failed to get session", http.StatusInternalServerError)
		return
	}

	if c.Query("state") != session.Values["state"] {
		http.Error(c.Writer, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// set the dynamic RedirectURL for the exchange
	authCfg := *oidcConfig
	authCfg.RedirectURL = GetBaseURL(c.Request) + "/auth/callback"

	oauth2Token, err := authCfg.Exchange(c.Request.Context(), c.Query("code"))
	if err != nil {
		http.Error(c.Writer, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(c.Writer, "No id_token field in oauth2 token.", http.StatusInternalServerError)
		return
	}

	idToken, err := oidcVerifier.Verify(c.Request.Context(), rawIDToken)
	if err != nil {
		http.Error(c.Writer, "Failed to verify ID Token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values["id_token"] = rawIDToken
	session.Values["user"] = claims.Email
	session.Options.MaxAge = sessionTTL
	session.Save(c.Request, c.Writer) //nolint:all

	logger.Logger.Info("User successfully authenticated", "user", claims.Email)
	http.Redirect(c.Writer, c.Request, "/", http.StatusFound)
}

// HandleLogout clears the user's session.
func HandleLogout(c *gin.Context) {
	session, err := sessionStore.Get(c.Request, "auth-session")
	if err == nil {
		session.Values["id_token"] = ""
		session.Values["user"] = ""
		session.Options.MaxAge = -1       // Expire the session cookie immediately
		session.Save(c.Request, c.Writer) //nolint:all
	}
	http.Redirect(c.Writer, c.Request, "/", http.StatusFound)
}
