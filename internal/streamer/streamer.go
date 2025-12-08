package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/parser"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/google/uuid"
)

const (
	TopicTelemetry = "telemetry"
	TopicGPU       = "gpu"
)

// Streamer reads CSV data and publishes to message queue
type Streamer struct {
	config     *Config
	queue      mq.MessageQueue
	gpuRepo    storage.GPURepository
	logger     *slog.Logger
	rowsSent   atomic.Int64
	errorCount atomic.Int64
}

// NewStreamer creates a new telemetry streamer
func NewStreamer(config *Config, queue mq.MessageQueue, gpuRepo storage.GPURepository, logger *slog.Logger) *Streamer {
	if logger == nil {
		logger = slog.Default()
	}

	return &Streamer{
		config:  config,
		queue:   queue,
		gpuRepo: gpuRepo,
		logger:  logger.With("component", "streamer", "instance_id", config.InstanceID),
	}
}

// Start begins streaming telemetry data
func (s *Streamer) Start(ctx context.Context) error {
	s.logger.Info("Starting telemetry streamer",
		"csv_path", s.config.CSVPath,
		"interval", s.config.StreamInterval,
		"loop_mode", s.config.LoopMode,
	)

	// Start statistics reporter
	go s.reportStats(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Streamer shutting down", "rows_sent", s.rowsSent.Load(), "errors", s.errorCount.Load())
			return ctx.Err()
		default:
			if err := s.streamFile(ctx); err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					return err
				}
				s.logger.Error("Error streaming file", "error", err)
				s.errorCount.Add(1)

				// If not in loop mode, exit on error
				if !s.config.LoopMode {
					return err
				}

				// Wait before retrying
				time.Sleep(5 * time.Second)
			}

			// If not in loop mode, exit after one pass
			if !s.config.LoopMode {
				s.logger.Info("Stream completed (loop mode disabled)", "rows_sent", s.rowsSent.Load())
				return nil
			}

			// Small delay before restarting loop
			s.logger.Info("Restarting stream from beginning", "rows_sent", s.rowsSent.Load())
			time.Sleep(time.Second)
		}
	}
}

// streamFile processes a single pass through the CSV file
func (s *Streamer) streamFile(ctx context.Context) error {
	file, err := os.Open(s.config.CSVPath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader, err := parser.NewStreamReader(file)
	if err != nil {
		return fmt.Errorf("failed to create stream reader: %w", err)
	}

	s.logger.Debug("Started streaming from file", "path", s.config.CSVPath)

	ticker := time.NewTicker(s.config.StreamInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			record, err := reader.Next()
			if err == io.EOF {
				s.logger.Debug("Reached end of file", "total_rows", reader.Row())
				return nil
			}
			if err != nil {
				s.logger.Warn("Failed to parse row", "row", reader.Row(), "error", err)
				s.errorCount.Add(1)
				continue
			}

			if err := s.processRecord(ctx, record); err != nil {
				s.logger.Warn("Failed to process record", "row", reader.Row(), "error", err)
				s.errorCount.Add(1)
				continue
			}

			s.rowsSent.Add(1)
		}
	}
}

// processRecord handles a single CSV record - store GPU and publish telemetry
func (s *Streamer) processRecord(ctx context.Context, record *parser.CSVRecord) error {
	// Store GPU info (idempotent - updates if exists)
	gpu := record.ToGPU()
	if err := s.gpuRepo.Store(gpu); err != nil {
		s.logger.Debug("Failed to store GPU", "uuid", gpu.UUID, "error", err)
		// Non-fatal - continue with telemetry
	}

	// Create telemetry point with current timestamp
	telemetry := record.ToTelemetryPoint()
	telemetry.Timestamp = time.Now()

	// Marshal to JSON
	payload, err := json.Marshal(telemetry)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	// Create message directly since payload is already JSON bytes
	msg := &mq.Message{
		ID:        uuid.New().String(),
		Topic:     TopicTelemetry,
		Payload:   payload,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
	msg.Headers["instance_id"] = s.config.InstanceID
	msg.Headers["gpu_uuid"] = telemetry.GPUUUID
	msg.Headers["metric_name"] = telemetry.MetricName
	msg.Metadata["hostname"] = record.Hostname
	msg.Metadata["device"] = record.Device

	// Publish to queue
	if err := s.queue.Publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	s.logger.Debug("Published telemetry",
		"gpu_uuid", telemetry.GPUUUID,
		"metric", telemetry.MetricName,
		"value", telemetry.Value,
	)

	return nil
}

// reportStats periodically logs statistics
func (s *Streamer) reportStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := s.queue.Stats()
			s.logger.Info("Streamer statistics",
				"rows_sent", s.rowsSent.Load(),
				"errors", s.errorCount.Load(),
				"queue_depth", stats.QueueDepth,
				"queue_published", stats.TotalPublished,
				"queue_delivered", stats.TotalDelivered,
			)
		}
	}
}

// Stats returns current streamer statistics
func (s *Streamer) Stats() StreamerStats {
	return StreamerStats{
		RowsSent:   s.rowsSent.Load(),
		ErrorCount: s.errorCount.Load(),
	}
}

// StreamerStats holds streamer statistics
type StreamerStats struct {
	RowsSent   int64
	ErrorCount int64
}
