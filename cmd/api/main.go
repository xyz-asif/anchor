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
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"github.com/xyz-asif/gotodo/internal/routes"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	docs "github.com/xyz-asif/gotodo/docs"
)

func main() {
	// Load config
	cfg := config.Load()

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
	router.Use(gin.Recovery())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS(cfg.FrontendURL))

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
