package streamer

import (
	"flag"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/config"
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
	cfg := &Config{}

	flag.StringVar(&cfg.CSVPath, "csv-path", config.GetEnv("CSV_PATH", config.DefaultStreamerCSVPath), "Path to CSV file")
	flag.DurationVar(&cfg.StreamInterval, "interval", config.GetEnvDuration("STREAM_INTERVAL", config.DefaultStreamInterval), "Interval between sending rows")
	flag.StringVar(&cfg.InstanceID, "instance-id", config.GetEnv("INSTANCE_ID", config.DefaultStreamerInstanceID), "Unique instance ID for this streamer")
	flag.IntVar(&cfg.QueueBufferSize, "queue-buffer", config.GetEnvInt("QUEUE_BUFFER_SIZE", config.DefaultQueueBufferSize), "Message queue buffer size")
	flag.IntVar(&cfg.QueueWorkers, "queue-workers", config.GetEnvInt("QUEUE_WORKERS", config.DefaultQueueWorkers), "Number of queue workers")
	flag.BoolVar(&cfg.LoopMode, "loop", config.GetEnvBool("LOOP_MODE", config.DefaultStreamerLoopMode), "Loop from beginning when EOF reached")

	flag.Parse()

	return cfg
}
