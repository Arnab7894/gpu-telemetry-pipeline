package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage/inmemory"

	_ "github.com/arnabghosh/gpu-metrics-streamer/docs/swagger" // Import generated swagger docs
)

// @title GPU Telemetry Pipeline API
// @version 1.0
// @description REST API for querying GPU telemetry data collected from DCGM metrics
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@gpu-telemetry.local

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api/v1

// @schemes http https
func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting API Gateway service",
		slog.String("service", "api-gateway"),
		slog.String("version", "1.0.0"),
	)

	// Load configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Initialize storage repositories
	var gpuRepo storage.GPURepository
	var telemetryRepo storage.TelemetryRepository

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI != "" {
		// Use MongoDB
		slog.Info("Using MongoDB storage", "mongodb_uri", mongoURI)

		mongoGPURepo, err := storage.NewMongoGPURepository(mongoURI, "telemetry", "gpus")
		if err != nil {
			slog.Error("Failed to connect to MongoDB for GPU", "error", err)
			os.Exit(1)
		}
		defer mongoGPURepo.Close(context.Background())
		gpuRepo = mongoGPURepo

		mongoTelemetryRepo, err := storage.NewMongoTelemetryRepository(mongoURI, "telemetry", "metrics")
		if err != nil {
			slog.Error("Failed to connect to MongoDB for telemetry", "error", err)
			os.Exit(1)
		}
		defer mongoTelemetryRepo.Close(context.Background())
		telemetryRepo = mongoTelemetryRepo

		slog.Info("Initialized MongoDB repositories",
			slog.String("database", "telemetry"),
			slog.String("gpu_collection", "gpus"),
			slog.String("telemetry_collection", "metrics"),
		)
	} else {
		// Use in-memory storage
		gpuRepo = inmemory.NewGPURepository()
		telemetryRepo = inmemory.NewTelemetryRepository()

		slog.Info("Initialized storage repositories",
			slog.String("gpu_repo", "in-memory"),
			slog.String("telemetry_repo", "in-memory"),
		)
	}

	// Create API router with handlers wired to repositories
	router := api.NewRouter(gpuRepo, telemetryRepo)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router.Engine(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("API Gateway initialized successfully",
		slog.String("port", port),
		slog.String("endpoints", "/api/v1/gpus, /api/v1/gpus/{uuid}/telemetry"),
	)

	// Start server in a goroutine
	go func() {
		slog.Info("Starting HTTP server", slog.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start HTTP server", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down API Gateway...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("API Gateway stopped gracefully")
}
