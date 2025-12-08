package collector

import (
	"flag"
	"os"
	"strconv"
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
	config := &Config{}

	flag.StringVar(&config.InstanceID, "instance-id", getEnv("INSTANCE_ID", "collector-1"), "Unique instance ID for this collector")
	flag.IntVar(&config.BatchSize, "batch-size", getEnvInt("BATCH_SIZE", 1), "Number of messages to batch before processing")
	flag.IntVar(&config.MaxConcurrentHandlers, "max-concurrent", getEnvInt("MAX_CONCURRENT_HANDLERS", 10), "Maximum concurrent message handlers")
	flag.IntVar(&config.QueueBufferSize, "queue-buffer", getEnvInt("QUEUE_BUFFER_SIZE", 1000), "Message queue buffer size")
	flag.IntVar(&config.QueueWorkers, "queue-workers", getEnvInt("QUEUE_WORKERS", 10), "Number of queue workers")

	flag.Parse()

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
