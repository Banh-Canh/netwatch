package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/boj/redistore"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/Banh-Canh/netwatch/docs"
	"github.com/Banh-Canh/netwatch/internal/handlers"
	"github.com/Banh-Canh/netwatch/internal/k8s"
	"github.com/Banh-Canh/netwatch/internal/middleware"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

// @title Netwatch API
// @description This is the API for the Netwatch application, providing endpoints to manage and view network access policies and requests.
// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and a valid OIDC ID token.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the Netwatch web server and API.",
	Long:  `Starts the user-facing web interface and WebSocket/API endpoints. This component handles user authentication and creates Access/ExternalAccess resources.`,
	Run: func(cmd *cobra.Command, args []string) {
		gin.SetMode(gin.ReleaseMode)

		if err := godotenv.Load(); err != nil {
			logger.Logger.Info("No .env file found, using environment variables.")
		}
		sessionSecret := os.Getenv("NETWATCH_SESSION_SECRET")
		oidcIssuerURL := os.Getenv("OIDC_ISSUER_URL")
		oidcClientID := os.Getenv("OIDC_CLIENT_ID")
		oidcClientSecret := os.Getenv("OIDC_CLIENT_SECRET")
		staticToken := os.Getenv("NETWATCH_API_TOKEN")
		ttlStr := os.Getenv("NETWATCH_SESSION_TTL")
		redisAddr := os.Getenv("REDIS_ADDR")
		redisUser := os.Getenv("REDIS_USERNAME")
		redisPass := os.Getenv("REDIS_PASSWORD")
		port := os.Getenv("NETWATCH_PORT")

		ttl, err := strconv.Atoi(ttlStr)
		if err != nil || ttl <= 0 {
			ttl = 3600
		}
		if redisAddr == "" {
			redisAddr = "localhost:6379"
		}

		if err := k8s.InitKubeClient(); err != nil {
			logger.Logger.Error("Fatal error initializing Kubernetes client", "error", err)
			os.Exit(1)
		}

		redisClient := redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Username: redisUser,
			Password: redisPass,
		})
		if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
			logger.Logger.Error("Could not connect to Redis for application logic", "error", err)
			os.Exit(1)
		}
		handlers.SetRedisClient(redisClient)

		go handlers.StartLogJanitor(context.Background(), redisClient, 5*time.Minute, time.Hour)

		store, err := redistore.NewRediStore(10, "tcp", redisAddr, redisUser, redisPass, []byte(sessionSecret))
		if err != nil {
			logger.Logger.Error("Could not create Redis session store", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := store.Close(); err != nil {
				logger.Logger.Error("Failed to close Redis session store cleanly", "error", err)
			}
		}()
		store.SetMaxAge(ttl)
		handlers.SetSessionStore(store)

		oidcHandlerConfig := handlers.OIDCConfig{
			IssuerURL:    oidcIssuerURL,
			ClientID:     oidcClientID,
			ClientSecret: oidcClientSecret,
			SessionTTL:   ttl,
		}
		if err := handlers.InitOIDC(oidcHandlerConfig); err != nil {
			logger.Logger.Error("Could not initialize OIDC handlers", "error", err)
			os.Exit(1)
		}
		if err := middleware.InitOIDCVerifier(oidcIssuerURL, oidcClientID); err != nil {
			logger.Logger.Error("Could not initialize API middleware OIDC verifier", "error", err)
			os.Exit(1)
		}

		router := gin.New()
		router.Use(gin.Recovery())
		router.Use(customLoggerMiddleware())

		router.LoadHTMLGlob("templates/*.html")
		router.Static("/static", "./static")
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

		router.GET("/", handlers.HandleMainPage(version))
		router.GET("/login", handlers.HandleLogin)
		router.GET("/logout", handlers.HandleLogout)
		router.GET("/auth/callback", handlers.HandleCallback)
		router.GET("/ws", handlers.HandleWebSocket)

		api := router.Group("/api")
		api.Use(middleware.AuthMiddleware(staticToken))
		{
			api.GET("/services", handlers.GetServices)
			api.GET("/active-accesses", handlers.GetActiveAccesses)
			api.GET("/logs", handlers.GetLogs)
			api.GET("/pending-requests", handlers.GetPendingRequests)
		}

		if port == "" {
			port = "3000"
		}
		addr := fmt.Sprintf(":%s", port)

		logger.Logger.Info("Netwatch web server starting", "address", addr)
		if err := router.Run(addr); err != nil {
			logger.Logger.Error("Could not start server", "error", err)
			os.Exit(1)
		}
	},
}

func customLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()
		latency := time.Since(start)
		logger.Logger.Info(
			"Request completed",
			"status",
			c.Writer.Status(),
			"method",
			c.Request.Method,
			"path",
			path,
			"query",
			query,
			"ip",
			c.ClientIP(),
			"latency",
			latency,
			"user-agent",
			c.Request.UserAgent(),
		)
	}
}
