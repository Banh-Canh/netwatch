// internal/handlers/handlers.go
package handlers

import (
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// This file contains truly shared variables and setup functions for the handlers package.

var (
	upgrader     = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	sessionStore sessions.Store
	redisClient  *redis.Client
	logKey       = "netwatch:activity_log"
)

// SetSessionStore injects the session store dependency.
func SetSessionStore(store sessions.Store) {
	sessionStore = store
}

// SetRedisClient injects the Redis client dependency.
func SetRedisClient(client *redis.Client) {
	redisClient = client
}
