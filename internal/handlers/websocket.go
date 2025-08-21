// internal/handlers/websocket.go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Banh-Canh/netwatch/internal/k8s"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// HandleWebSocket manages the WebSocket lifecycle and dispatches commands.
func HandleWebSocket(c *gin.Context) {
	idToken, err := getUserIdToken(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	userInfo, err := k8s.GetUserInfoFromToken(c.Request.Context(), idToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token: " + err.Error()})
		return
	}

	sanitizedUsername := strings.ReplaceAll(userInfo.Email, "@", "-")
	sanitizedUsername = strings.ReplaceAll(sanitizedUsername, ".", "-")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Logger.Error("Failed to upgrade connection", "error", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Logger.Error("Failed to close cleanly", "error", err)
		}
	}()
	var connMu sync.Mutex

	logAndBroadcast := func(entry LogEntry) {
		entry.Timestamp = time.Now().UnixMilli()
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			logger.Logger.Error("Failed to marshal log entry for Redis", "error", err)
			return
		}
		ctx := context.Background()
		if err := redisClient.ZAdd(ctx, logKey, redis.Z{
			Score:  float64(entry.Timestamp),
			Member: entryJSON,
		}).Err(); err != nil {
			logger.Logger.Error("Failed to save log entry to Redis", "error", err)
		}

		connMu.Lock()
		defer connMu.Unlock()
		if err := conn.WriteJSON(entry); err != nil {
			logger.Logger.Warn("Could not write JSON to WebSocket", "error", err)
		}
	}

	sendError := func(msg string, err error, logType string) {
		var fullMsg string
		if err != nil {
			logger.Logger.Error(msg, "error", err, "user", userInfo.Email)
			fullMsg = fmt.Sprintf("REQUEST FAILED: %s - %s", msg, err.Error())
		} else {
			logger.Logger.Warn(msg, "user", userInfo.Email)
			fullMsg = fmt.Sprintf("REQUEST FAILED: %s", msg)
		}
		logAndBroadcast(LogEntry{
			Payload:   fullMsg,
			ClassName: "log-error", LogType: logType, Type: "applyResult",
		})
		logAndBroadcast(LogEntry{
			Payload:   "--- Request failed ---",
			ClassName: "log-error", LogType: logType, Type: "applyComplete",
		})
	}

	// Create a command processor for this WebSocket session.
	processor := &webSocketCommandProcessor{
		ctx:               c.Request.Context(),
		idToken:           idToken,
		userInfo:          userInfo,
		sanitizedUsername: sanitizedUsername,
		logAndBroadcast:   logAndBroadcast,
		sendError:         sendError,
	}

	for {
		var payload webSocketPayload
		if err := conn.ReadJSON(&payload); err != nil {
			logger.Logger.Info("Client disconnected", "error", err)
			break
		}

		switch payload.Command {
		case "requestClusterAccess":
			processor.handleRequestClusterAccess(payload)
		case "requestExternalAccess":
			processor.handleRequestExternalAccess(payload)
		case "submitAccessRequest":
			processor.handleSubmitAccessRequest(payload)
		case "approveAccessRequest":
			processor.handleApproveAccessRequest(payload)
		case "denyAccessRequest":
			processor.handleDenyAccessRequest(payload)
		case "revokeClusterAccess":
			processor.handleRevokeClusterAccess(payload)
		case "revokeExternalAccess":
			processor.handleRevokeExternalAccess(payload)
		default:
			logger.Logger.Warn("Received unknown WebSocket command", "command", payload.Command)
		}
	}
}

// getUserIdToken retrieves the OIDC ID token from the Gin context or session.
func getUserIdToken(c *gin.Context) (string, error) {
	if token, exists := c.Get("id_token"); exists {
		if tokenStr, ok := token.(string); ok {
			return tokenStr, nil
		}
	}
	session, err := sessionStore.Get(c.Request, "auth-session")
	if err != nil {
		return "", errors.New("could not retrieve session")
	}
	if idToken, ok := session.Values["id_token"].(string); ok && idToken != "" {
		return idToken, nil
	}
	return "", errors.New("user not authenticated")
}

// GetBaseURL constructs the base URL of the application. Mostly useful for the redirect_uri
func GetBaseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	host := r.Header.Get("X-Forwarded-Host")

	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if host == "" {
		host = r.Host
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
