package config

import "time"

// Default configuration values for all services
const (
	// API Gateway defaults
	DefaultAPIPort         = "8080"
	DefaultReadTimeout     = 15 * time.Second
	DefaultWriteTimeout    = 15 * time.Second
	DefaultIdleTimeout     = 60 * time.Second
	DefaultShutdownTimeout = 30 * time.Second

	// Message Queue defaults
	DefaultQueueBufferSize = 1000
	DefaultQueueWorkers    = 10

	// Collector defaults
	DefaultCollectorBatchSize    = 1
	DefaultMaxConcurrentHandlers = 10
	DefaultCollectorInstanceID   = "collector-1"

	// Streamer defaults
	DefaultStreamerCSVPath    = "/app/data/metrics.csv"
	DefaultStreamInterval     = 100 * time.Millisecond
	DefaultStreamerInstanceID = "streamer-1"
	DefaultStreamerLoopMode   = true

	// Queue Service defaults
	DefaultQueueServicePort = 8080

	// MongoDB defaults
	DefaultMongoDatabase          = "telemetry"
	DefaultMongoGPUCollection     = "gpus"
	DefaultMongoMetricsCollection = "metrics"
)
