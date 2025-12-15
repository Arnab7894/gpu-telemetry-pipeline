package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/streamer"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	config := streamer.LoadConfig()

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
		"consumer_group", "streamers",
		"consumer_id", config.InstanceID,
	)

	queue := mq.NewHTTPQueueClient(mq.HTTPQueueConfig{
		BaseURL:       queueServiceURL,
		ConsumerGroup: "streamers",
		ConsumerID:    config.InstanceID,
	}, logger)

	logger.Info("Loaded configuration",
		"csv_path", config.CSVPath,
		"instance_id", config.InstanceID,
		"interval", config.StreamInterval,
		"loop_mode", config.LoopMode,
		"queue_type", "http",
	)

	// Initialize batch lock (Redis-based distributed lock)
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379" // Default for Kubernetes
	}

	batchLock, err := streamer.NewBatchLock(redisURL, config.InstanceID, logger)
	if err != nil {
		logger.Error("Failed to initialize batch lock", "error", err)
		os.Exit(1)
	}
	defer batchLock.Close()

	logger.Info("Initialized batch lock",
		"redis_url", redisURL,
		"instance_id", config.InstanceID,
	)

	// Create streamer
	telemetryStreamer := streamer.NewStreamer(config, queue, batchLock, logger)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start streamer in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- telemetryStreamer.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()

		// Wait for streamer to finish
		<-errChan

	case err := <-errChan:
		if err != nil && err != context.Canceled {
			logger.Error("Streamer error", "error", err)
			os.Exit(1)
		}
	}

	// Final statistics
	stats := telemetryStreamer.Stats()

	logger.Info("Shutdown complete",
		"rows_sent", stats.RowsSent,
		"errors", stats.ErrorCount,
	)

	fmt.Println("Telemetry streamer shut down gracefully")
}
