package collector

import (
	"flag"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/config"
)

// Config holds the configuration for the telemetry collector
type Config struct {
	InstanceID            string
	BatchSize             int
	MaxConcurrentHandlers int
	QueueBufferSize       int
	QueueWorkers          int
}

// LoadConfig loads configuration from environment variables and command-line flags
func LoadConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.InstanceID, "instance-id", config.GetEnv("INSTANCE_ID", config.DefaultCollectorInstanceID), "Unique instance ID for this collector")
	flag.IntVar(&cfg.BatchSize, "batch-size", config.GetEnvInt("BATCH_SIZE", config.DefaultCollectorBatchSize), "Number of messages to batch before processing")
	flag.IntVar(&cfg.MaxConcurrentHandlers, "max-concurrent", config.GetEnvInt("MAX_CONCURRENT_HANDLERS", config.DefaultMaxConcurrentHandlers), "Maximum concurrent message handlers")
	flag.IntVar(&cfg.QueueBufferSize, "queue-buffer", config.GetEnvInt("QUEUE_BUFFER_SIZE", config.DefaultQueueBufferSize), "Message queue buffer size")
	flag.IntVar(&cfg.QueueWorkers, "queue-workers", config.GetEnvInt("QUEUE_WORKERS", config.DefaultQueueWorkers), "Number of queue workers")

	flag.Parse()

	return cfg
}
