package api

import (
	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/handlers"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/middleware"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/gin-gonic/gin"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Router manages API routing and handlers
type Router struct {
	engine           *gin.Engine
	gpuHandler       *handlers.GPUHandler
	telemetryHandler *handlers.TelemetryHandler
}

// NewRouter creates a new API router with all handlers initialized
func NewRouter(gpuRepo storage.GPURepository, telemetryRepo storage.TelemetryRepository) *Router {
	router := &Router{
		engine:           gin.New(),
		gpuHandler:       handlers.NewGPUHandler(gpuRepo),
		telemetryHandler: handlers.NewTelemetryHandler(telemetryRepo, gpuRepo),
	}

	router.setupMiddleware()
	router.setupRoutes()

	return router
}

// setupMiddleware configures global middleware
func (r *Router) setupMiddleware() {
	// Logging middleware
	r.engine.Use(middleware.LoggingMiddleware())

	// Error handling middleware
	r.engine.Use(middleware.ErrorHandlerMiddleware())

	// Recovery middleware (catch panics)
	r.engine.Use(gin.Recovery())
}

// setupRoutes configures all API routes
func (r *Router) setupRoutes() {
	// Health check
	r.engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})

	// Swagger UI - serves OpenAPI documentation at /swagger/index.html
	r.engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1 routes
	v1 := r.engine.Group("/api/v1")
	{
		// GPU endpoints
		gpus := v1.Group("/gpus")
		{
			gpus.GET("", r.gpuHandler.ListGPUs)
			gpus.GET("/:uuid", r.gpuHandler.GetGPU)
			gpus.GET("/:uuid/telemetry", r.telemetryHandler.GetGPUTelemetry)
		}
	}
}

// Engine returns the underlying Gin engine
func (r *Router) Engine() *gin.Engine {
	return r.engine
}

// Run starts the HTTP server
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}
