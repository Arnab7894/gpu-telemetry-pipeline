package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/queueservice"
)

func main() {
	// Parse command-line flags
	port := flag.Int("port", 8080, "HTTP server port")
	redisURL := flag.String("redis-url", "redis://localhost:6379", "Redis connection URL")
	visibilityTimeout := flag.Duration("visibility-timeout", 5*time.Minute, "Message visibility timeout")
	maxRetries := flag.Int("max-retries", 3, "Maximum delivery attempts before moving to DLQ")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Override with environment variables
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", port)
	}
	if envRedisURL := os.Getenv("REDIS_URL"); envRedisURL != "" {
		*redisURL = envRedisURL
	}
	if envVisibilityTimeout := os.Getenv("VISIBILITY_TIMEOUT"); envVisibilityTimeout != "" {
		if d, err := time.ParseDuration(envVisibilityTimeout); err == nil {
			*visibilityTimeout = d
		}
	}
	if envMaxRetries := os.Getenv("MAX_RETRIES"); envMaxRetries != "" {
		fmt.Sscanf(envMaxRetries, "%d", maxRetries)
	}
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		*logLevel = envLogLevel
	}

	// Configure logger
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger)

	logger.Info("Starting Queue Service",
		"port", *port,
		"redis_url", *redisURL,
		"visibility_timeout", *visibilityTimeout,
		"max_retries", *maxRetries,
		"log_level", *logLevel,
	)

	// Create Redis-backed queue
	queue, err := queueservice.NewRedisQueue(*redisURL, *visibilityTimeout, *maxRetries, logger)
	if err != nil {
		logger.Error("Failed to create Redis queue", "error", err)
		os.Exit(1)
	}
	defer queue.Close()

	logger.Info("Redis queue initialized successfully")

	// Create HTTP server
	httpServer := queueservice.NewHTTPServer(queue, *port, logger)

	// Start HTTP server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- httpServer.Start()
	}()

	logger.Info("Queue Service started successfully",
		"http_addr", fmt.Sprintf(":%d", *port),
	)

	// Wait for interrupt signal or server error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("Server error", "error", err)
		os.Exit(1)

	case sig := <-shutdown:
		logger.Info("Shutdown signal received", "signal", sig.String())

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown HTTP server
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Error("Failed to shutdown HTTP server gracefully", "error", err)
			os.Exit(1)
		}

		logger.Info("Queue Service stopped gracefully")
	}
}
