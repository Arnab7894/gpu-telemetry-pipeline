package streamer

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the telemetry streamer
type Config struct {
	CSVPath         string
	StreamInterval  time.Duration
	InstanceID      string
	QueueBufferSize int
	QueueWorkers    int
	LoopMode        bool
}

// LoadConfig loads configuration from environment variables and command-line flags
func LoadConfig() *Config {
	config := &Config{}

	flag.StringVar(&config.CSVPath, "csv-path", getEnv("CSV_PATH", "/app/data/metrics.csv"), "Path to CSV file")
	flag.DurationVar(&config.StreamInterval, "interval", getEnvDuration("STREAM_INTERVAL", 100*time.Millisecond), "Interval between sending rows")
	flag.StringVar(&config.InstanceID, "instance-id", getEnv("INSTANCE_ID", "streamer-1"), "Unique instance ID for this streamer")
	flag.IntVar(&config.QueueBufferSize, "queue-buffer", getEnvInt("QUEUE_BUFFER_SIZE", 1000), "Message queue buffer size")
	flag.IntVar(&config.QueueWorkers, "queue-workers", getEnvInt("QUEUE_WORKERS", 10), "Number of queue workers")
	flag.BoolVar(&config.LoopMode, "loop", getEnvBool("LOOP_MODE", true), "Loop from beginning when EOF reached")

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

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
