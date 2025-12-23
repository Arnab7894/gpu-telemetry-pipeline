package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/collector"
	appconfig "github.com/arnabghosh/gpu-metrics-streamer/internal/config"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage/mongodb"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	config := collector.LoadConfig()

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize HTTP Queue Client (connects to Queue Service)
	queueServiceURL := os.Getenv("QUEUE_SERVICE_URL")
	if queueServiceURL == "" {
		queueServiceURL = "http://queue-service:8080" // Default for Kubernetes deployment
	}

	logger.Info("Using HTTP queue client",
		"queue_service_url", queueServiceURL,
		"consumer_group", "collectors",
		"consumer_id", config.InstanceID,
	)

	queue := mq.NewHTTPQueueClient(mq.HTTPQueueConfig{
		BaseURL:       queueServiceURL,
		ConsumerGroup: "collectors",
		ConsumerID:    config.InstanceID,
	}, logger)

	// TODO: Remove this as it is not needed for consumers
	if err := queue.Start(ctx); err != nil {
		logger.Error("Failed to start message queue", "error", err)
		os.Exit(1)
	}

	logger.Info("Loaded configuration",
		"instance_id", config.InstanceID,
		"batch_size", config.BatchSize,
		"max_concurrent", config.MaxConcurrentHandlers,
		"queue_type", "http",
	)

	// Initialize repositories
	var telemetryRepo storage.TelemetryRepository
	var gpuRepo storage.GPURepository

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI != "" {
		// Use MongoDB
		logger.Info("Using MongoDB storage", "mongodb_uri", mongoURI)

		mongoTelemetryRepo, err := mongodb.NewTelemetryRepository(mongoURI, appconfig.DefaultMongoDatabase, appconfig.DefaultMongoMetricsCollection)
		if err != nil {
			logger.Error("Failed to connect to MongoDB for telemetry", "error", err)
			os.Exit(1)
		}
		defer mongoTelemetryRepo.Close(context.Background())
		telemetryRepo = mongoTelemetryRepo

		mongoGPURepo, err := mongodb.NewGPURepository(mongoURI, appconfig.DefaultMongoDatabase, appconfig.DefaultMongoGPUCollection)
		if err != nil {
			logger.Error("Failed to connect to MongoDB for GPU", "error", err)
			os.Exit(1)
		}
		defer mongoGPURepo.Close(context.Background())
		gpuRepo = mongoGPURepo
	} else {
		logger.Error("MongoDB URI not configured. Set MONGODB_URI environment variable.")
		os.Exit(1)
	}

	// Create collector
	telemetryCollector := collector.NewCollector(config, queue, telemetryRepo, gpuRepo, logger)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start collector in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- telemetryCollector.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()

		// Wait for collector to finish
		<-errChan

	case err := <-errChan:
		if err != nil && err != context.Canceled {
			logger.Error("Collector error", "error", err)
			os.Exit(1)
		}
	}

	// Final statistics
	stats := telemetryCollector.Stats()

	logger.Info("Shutdown complete",
		"messages_processed", stats.MessagesProcessed,
		"messages_errors", stats.MessagesErrors,
		"gpus_stored", stats.GPUsStored,
		"telemetry_stored", stats.TelemetryStored,
	)

	fmt.Println("Telemetry collector shut down gracefully")
}
