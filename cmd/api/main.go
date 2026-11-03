// ================== cmd/api/main.go ==================
//
// @title GoTodo API
// @version 1.0
// @description A RESTful API for managing todos with JWT authentication
// @host localhost:8080
// @BasePath /api/v1
// @schemes http
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer <token>"
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/database"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"github.com/xyz-asif/gotodo/internal/pkg/ratelimit"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"github.com/xyz-asif/gotodo/internal/routes"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	docs "github.com/xyz-asif/gotodo/docs"
)

func main() {
	// Load config and refuse to start with unsafe settings in production
	cfg := config.Load()
	warnings, err := cfg.Validate()
	if err != nil {
		log.Fatal(err)
	}
	for _, w := range warnings {
		log.Printf("CONFIG WARNING: %s", w)
	}
	if cfg.IsProduction() && len(cfg.TrustedProxies) == 0 {
		log.Printf("CONFIG WARNING: TRUSTED_PROXIES is empty. If this server runs behind a load balancer or " +
			"reverse proxy, every client will share the proxy's IP and its rate limits; set TRUSTED_PROXIES.")
	}

	// Configure Swagger metadata at runtime
	docs.SwaggerInfo.Title = "GoTodo API"
	docs.SwaggerInfo.Description = "A RESTful API for managing todos with JWT authentication"
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Host = "localhost:" + cfg.Port
	docs.SwaggerInfo.BasePath = "/api/v1"
	docs.SwaggerInfo.Schemes = []string{"http"}

	// Connect to MongoDB
	db, err := database.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer db.Disconnect(context.Background())
	//If we are running in production, be quiet and stop logging so much.
	// Setup Gin
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Only trust X-Forwarded-For from configured proxies (e.g. your load balancer).
	// Otherwise clients could spoof their IP and bypass rate limits.
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		log.Fatal("Invalid TRUSTED_PROXIES:", err)
	}

	router.Use(gin.Recovery())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS(cfg.FrontendURL))

	// Rate limiting (in-memory, per client IP; per instance if you run several).
	// Must be registered before the routes below.
	globalLimiter := ratelimit.New(cfg.RateLimitPerMinute, time.Minute)
	authLimiter := ratelimit.New(cfg.AuthRateLimitPerMinute, time.Minute)
	uploadLimiter := ratelimit.New(cfg.UploadRateLimitPerMinute, time.Minute)
	for _, l := range []*ratelimit.RateLimiter{globalLimiter, authLimiter, uploadLimiter} {
		l.StartCleanup(5 * time.Minute)
	}
	router.Use(ratelimit.Middleware(globalLimiter))
	router.Use(ratelimit.RouteLimits(map[string]*ratelimit.RateLimiter{
		// Login / token endpoints
		"POST /api/v1/auth/google":    authLimiter,
		"POST /api/v1/auth/dev-login": authLimiter,
		"POST /api/v1/auth/refresh":   authLimiter,
		// Uploads and endpoints that fetch remote URLs
		"POST /api/v1/media/upload":               uploadLimiter,
		"GET /api/v1/media/preview":               uploadLimiter,
		"POST /api/v1/anchors/:id/items/upload":   uploadLimiter,
		"POST /api/v1/anchors/:id/items":          uploadLimiter,
		"PATCH /api/v1/anchors/:id/items/:itemId": uploadLimiter, // re-fetches link metadata
		"POST /api/v1/users/me/profile-picture":   uploadLimiter,
		"POST /api/v1/users/me/cover-image":       uploadLimiter,
	}))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		response.Success(c, map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	// Swagger documentation (modern UI configs)
	router.GET(
		"/swagger/*any",
		ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.URL("/swagger/doc.json"),
			ginSwagger.DeepLinking(true),
			ginSwagger.DefaultModelsExpandDepth(-1),
			ginSwagger.DocExpansion("none"),
			ginSwagger.PersistAuthorization(true),
		),
	)

	// Register all routes
	routes.SetupRoutes(router, db.Database, cfg)

	// config server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}
	// start the server
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Failed to start server:", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// if it takes less than 5 sec clear all the things so that we dont use or holding onto resources unnecessarily.
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
