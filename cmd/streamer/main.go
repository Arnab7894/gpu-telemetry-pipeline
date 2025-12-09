package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage/inmemory"
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

	// Initialize Redis message queue
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://redis:6379" // Default for Kubernetes deployment
	}

	logger.Info("Using Redis queue",
		"redis_url", redisURL,
		"consumer_group", "streamers",
		"consumer_id", config.InstanceID,
	)

	queue, err := mq.NewRedisQueue(mq.RedisQueueConfig{
		RedisURL:      redisURL,
		ConsumerGroup: "streamers",
		ConsumerID:    config.InstanceID,
	}, logger)

	if err != nil {
		logger.Error("Failed to create Redis queue", "error", err)
		os.Exit(1)
	}

	if err := queue.Start(ctx); err != nil {
		logger.Error("Failed to start message queue", "error", err)
		os.Exit(1)
	}

	logger.Info("Loaded configuration",
		"csv_path", config.CSVPath,
		"instance_id", config.InstanceID,
		"interval", config.StreamInterval,
		"loop_mode", config.LoopMode,
		"queue_type", "redis",
	)

	// Initialize GPU repository
	gpuRepo := inmemory.NewGPURepository()

	// Create streamer
	telemetryStreamer := streamer.NewStreamer(config, queue, gpuRepo, logger)

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
	queueStats := queue.Stats()

	logger.Info("Shutdown complete",
		"rows_sent", stats.RowsSent,
		"errors", stats.ErrorCount,
		"queue_published", queueStats.TotalPublished,
		"queue_delivered", queueStats.TotalDelivered,
	)

	fmt.Println("Telemetry streamer shut down gracefully")
}
