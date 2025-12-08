package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/queueservice"
)

func main() {
	// Configuration
	var (
		port              int
		visibilityTimeout time.Duration
		maxRetries        int
		bufferSize        int
	)

	flag.IntVar(&port, "port", getEnvInt("PORT", 8080), "HTTP server port")
	flag.DurationVar(&visibilityTimeout, "visibility-timeout", getEnvDuration("VISIBILITY_TIMEOUT", 5*time.Minute), "Message visibility timeout")
	flag.IntVar(&maxRetries, "max-retries", getEnvInt("MAX_RETRIES", 3), "Maximum delivery retries before dead letter")
	flag.IntVar(&bufferSize, "buffer-size", getEnvInt("BUFFER_SIZE", 1000), "Topic buffer size")
	flag.Parse()

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting Queue Service",
		"port", port,
		"visibility_timeout", visibilityTimeout,
		"max_retries", maxRetries,
		"buffer_size", bufferSize,
	)

	// Create queue
	queue := queueservice.NewCompetingConsumerQueue(queueservice.Config{
		BufferSize:        bufferSize,
		VisibilityTimeout: visibilityTimeout,
		MaxRetries:        maxRetries,
	}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start queue background processes
	if err := queue.Start(ctx); err != nil {
		logger.Error("Failed to start queue", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	httpServer := queueservice.NewHTTPServer(queue, port, logger)

	// Start HTTP server in goroutine
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Error("HTTP server failed", "error", err)
			cancel()
		}
	}()

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Queue Service started successfully",
		"endpoints", map[string]string{
			"publish":   "POST /api/v1/queue/publish",
			"subscribe": "GET /api/v1/queue/subscribe",
			"ack":       "POST /api/v1/queue/ack",
			"nack":      "POST /api/v1/queue/nack",
			"stats":     "GET /api/v1/queue/stats",
			"health":    "GET /health",
		},
	)

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutdown signal received")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	// Shutdown queue
	if err := queue.Shutdown(shutdownCtx); err != nil {
		logger.Error("Queue shutdown error", "error", err)
	}

	// Log final statistics
	stats := queue.Stats()
	logger.Info("Queue Service shutdown complete",
		"final_stats", stats,
	)
}

// Helper functions
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
