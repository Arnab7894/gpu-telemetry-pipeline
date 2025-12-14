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
	batchLock  *BatchLock // Distributed lock for CSV processing
	logger     *slog.Logger
	rowsSent   atomic.Int64
	errorCount atomic.Int64
}

// NewStreamer creates a new telemetry streamer
func NewStreamer(config *Config, queue mq.MessageQueue, batchLock *BatchLock, logger *slog.Logger) *Streamer {
	if logger == nil {
		logger = slog.Default()
	}

	return &Streamer{
		config:    config,
		queue:     queue,
		batchLock: batchLock,
		logger:    logger.With("component", "streamer", "instance_id", config.InstanceID),
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
			// Acquire distributed lock before processing CSV
			if s.batchLock != nil {
				s.logger.Info("Attempting to acquire CSV batch lock...")
				if err := s.batchLock.AcquireLock(ctx); err != nil {
					if err == context.Canceled || err == context.DeadlineExceeded {
						return err
					}
					s.logger.Error("Failed to acquire batch lock", "error", err)
					time.Sleep(5 * time.Second)
					continue
				}

				// Ensure lock is released after processing
				defer func() {
					if releaseErr := s.batchLock.ReleaseLock(context.Background()); releaseErr != nil {
						s.logger.Warn("Failed to release batch lock", "error", releaseErr)
					}
				}()
			}

			// Process CSV batch atomically
			if err := s.streamFile(ctx); err != nil {
				// Release lock before handling error
				if s.batchLock != nil {
					s.batchLock.ReleaseLock(context.Background())
				}

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
				continue
			}

			// Release lock after successful batch processing
			if s.batchLock != nil {
				if err := s.batchLock.ReleaseLock(ctx); err != nil {
					s.logger.Warn("Failed to release batch lock", "error", err)
				}
			}

			// If not in loop mode, exit after one pass
			if !s.config.LoopMode {
				s.logger.Info("Stream completed (loop mode disabled)", "rows_sent", s.rowsSent.Load())
				return nil
			}

			// Small delay before restarting loop
			s.logger.Info("Batch complete, waiting before next batch",
				"rows_sent", s.rowsSent.Load(),
				"wait_time", "5s",
			)
			time.Sleep(5 * time.Second)
		}
	}
}

// streamFile processes a single pass through the CSV file
// Reads entire CSV atomically and assigns same timestamp to all records in the batch
func (s *Streamer) streamFile(ctx context.Context) error {
	file, err := os.Open(s.config.CSVPath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file %s: %w", s.config.CSVPath, err)
	}
	defer file.Close()

	reader, err := parser.NewStreamReader(file)
	if err != nil {
		return fmt.Errorf("failed to create stream reader: %w", err)
	}

	s.logger.Info("Started reading CSV batch", "path", s.config.CSVPath)

	// Single batch timestamp for ALL records in this CSV read
	batchTimestamp := time.Now()
	batchID := batchTimestamp.Format(time.RFC3339Nano)

	s.logger.Info("Batch processing started",
		"batch_id", batchID,
		"timestamp", batchTimestamp,
	)

	// Read all records from CSV into memory
	var records []*parser.CSVRecord
	rowCount := 0
	for {
		record, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.logger.Warn("Failed to parse row", "row", reader.Row(), "error", err)
			s.errorCount.Add(1)
			continue
		}
		records = append(records, record)
		rowCount++
	}

	s.logger.Info("CSV batch read complete",
		"batch_id", batchID,
		"total_records", rowCount,
	)

	// Process and publish entire batch with same timestamp
	publishedCount := 0
	errorCount := 0

	for _, record := range records {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := s.processRecord(ctx, record, batchTimestamp, batchID); err != nil {
				s.logger.Warn("Failed to process record",
					"gpu_uuid", record.UUID,
					"metric", record.MetricName,
					"error", err,
				)
				errorCount++
				continue
			}
			publishedCount++
			s.rowsSent.Add(1)
		}
	}

	s.logger.Info("Batch processing complete",
		"batch_id", batchID,
		"published", publishedCount,
		"errors", errorCount,
		"total_records", rowCount,
	)

	return nil
}

// processRecord handles a single CSV record - publish telemetry with GPU metadata
// All records in a batch share the same timestamp
func (s *Streamer) processRecord(ctx context.Context, record *parser.CSVRecord, batchTimestamp time.Time, batchID string) error {
	// Create telemetry point with batch timestamp (NOT time.Now())
	telemetry := record.ToTelemetryPoint()
	telemetry.Timestamp = batchTimestamp // All records in batch get same timestamp
	telemetry.BatchID = batchID          // Track which batch this belongs to

	// Marshal to JSON
	payload, err := json.Marshal(telemetry)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	// Create message with complete GPU metadata
	msg := &mq.Message{
		ID:        uuid.New().String(),
		Topic:     TopicTelemetry,
		Payload:   payload,
		Headers:   make(map[string]string),
		Timestamp: batchTimestamp,
		Metadata:  make(map[string]interface{}),
	}
	msg.Headers["instance_id"] = s.config.InstanceID
	msg.Headers["gpu_uuid"] = telemetry.GPUUUID
	msg.Headers["metric_name"] = telemetry.MetricName
	msg.Headers["batch_id"] = batchID

	// Include ALL GPU metadata in message for collector
	msg.Metadata["hostname"] = record.Hostname
	msg.Metadata["device"] = record.Device
	msg.Metadata["gpu_index"] = record.GPUID
	msg.Metadata["model_name"] = record.ModelName
	msg.Metadata["container"] = record.Container
	msg.Metadata["pod"] = record.Pod
	msg.Metadata["namespace"] = record.Namespace

	// Publish to queue
	if err := s.queue.Publish(ctx, msg); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	s.logger.Debug("Published telemetry",
		"batch_id", batchID,
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
