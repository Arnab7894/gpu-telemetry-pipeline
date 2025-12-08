package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
)

const (
	TopicTelemetry = "telemetry"
)

// Collector subscribes to message queue and stores telemetry data
type Collector struct {
	config        *Config
	queue         mq.MessageQueue
	telemetryRepo storage.TelemetryRepository
	gpuRepo       storage.GPURepository
	logger        *slog.Logger

	// Statistics
	messagesProcessed atomic.Int64
	messagesErrors    atomic.Int64
	gpusStored        atomic.Int64
	telemetryStored   atomic.Int64
}

// NewCollector creates a new telemetry collector
func NewCollector(
	config *Config,
	queue mq.MessageQueue,
	telemetryRepo storage.TelemetryRepository,
	gpuRepo storage.GPURepository,
	logger *slog.Logger,
) *Collector {
	if logger == nil {
		logger = slog.Default()
	}

	return &Collector{
		config:        config,
		queue:         queue,
		telemetryRepo: telemetryRepo,
		gpuRepo:       gpuRepo,
		logger:        logger.With("component", "collector", "instance_id", config.InstanceID),
	}
}

// Start begins collecting telemetry messages
func (c *Collector) Start(ctx context.Context) error {
	c.logger.Info("Starting telemetry collector",
		"batch_size", c.config.BatchSize,
		"max_concurrent", c.config.MaxConcurrentHandlers,
	)

	// Subscribe to telemetry topic
	if err := c.queue.Subscribe(ctx, TopicTelemetry, c.handleMessage); err != nil {
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	c.logger.Info("Subscribed to telemetry topic")

	// Start statistics reporter
	go c.reportStats(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	c.logger.Info("Collector shutting down",
		"messages_processed", c.messagesProcessed.Load(),
		"errors", c.messagesErrors.Load(),
	)

	// Unsubscribe
	if err := c.queue.Unsubscribe(TopicTelemetry); err != nil {
		c.logger.Warn("Failed to unsubscribe", "error", err)
	}

	return ctx.Err()
}

// handleMessage processes a single telemetry message
func (c *Collector) handleMessage(ctx context.Context, msg *mq.Message) error {
	c.messagesProcessed.Add(1)

	// Parse telemetry point from message payload
	var telemetry domain.TelemetryPoint
	if err := json.Unmarshal(msg.Payload, &telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Warn("Failed to unmarshal telemetry",
			"message_id", msg.ID,
			"error", err,
		)
		// Return nil to acknowledge the message (don't requeue malformed data)
		return nil
	}

	// Validate telemetry data
	if err := c.validateTelemetry(&telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Warn("Invalid telemetry data",
			"message_id", msg.ID,
			"gpu_uuid", telemetry.GPUUUID,
			"error", err,
		)
		return nil
	}

	// Extract GPU metadata from message headers/metadata
	gpu := c.extractGPUFromMessage(msg, &telemetry)
	if gpu != nil {
		// Store GPU metadata (idempotent)
		if err := c.gpuRepo.Store(gpu); err != nil {
			c.logger.Debug("Failed to store GPU metadata",
				"gpu_uuid", gpu.UUID,
				"error", err,
			)
			// Non-fatal - continue with telemetry storage
		} else {
			c.gpusStored.Add(1)
		}
	}

	// Store telemetry point
	if err := c.telemetryRepo.Store(&telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Error("Failed to store telemetry",
			"message_id", msg.ID,
			"gpu_uuid", telemetry.GPUUUID,
			"metric", telemetry.MetricName,
			"error", err,
		)
		// Return error to potentially requeue
		return fmt.Errorf("failed to store telemetry: %w", err)
	}

	c.telemetryStored.Add(1)

	c.logger.Debug("Stored telemetry",
		"message_id", msg.ID,
		"gpu_uuid", telemetry.GPUUUID,
		"metric", telemetry.MetricName,
		"value", telemetry.Value,
	)

	return nil
}

// validateTelemetry validates a telemetry point
func (c *Collector) validateTelemetry(t *domain.TelemetryPoint) error {
	if t.GPUUUID == "" {
		return fmt.Errorf("missing GPU UUID")
	}
	if t.MetricName == "" {
		return fmt.Errorf("missing metric name")
	}
	if t.Timestamp.IsZero() {
		return fmt.Errorf("missing timestamp")
	}
	return nil
}

// extractGPUFromMessage extracts GPU metadata from message headers/metadata
func (c *Collector) extractGPUFromMessage(msg *mq.Message, telemetry *domain.TelemetryPoint) *domain.GPU {
	// Extract from message metadata
	hostname, _ := msg.Metadata["hostname"].(string)
	device, _ := msg.Metadata["device"].(string)

	// Only create GPU if we have meaningful metadata
	if hostname == "" && device == "" {
		return nil
	}

	return &domain.GPU{
		UUID:     telemetry.GPUUUID,
		DeviceID: device,
		Hostname: hostname,
		// Other fields would come from message metadata if available
	}
}

// reportStats periodically logs statistics
func (c *Collector) reportStats(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queueStats := c.queue.Stats()
			c.logger.Info("Collector statistics",
				"messages_processed", c.messagesProcessed.Load(),
				"messages_errors", c.messagesErrors.Load(),
				"gpus_stored", c.gpusStored.Load(),
				"telemetry_stored", c.telemetryStored.Load(),
				"queue_delivered", queueStats.TotalDelivered,
				"queue_depth", queueStats.QueueDepth,
			)
		}
	}
}

// Stats returns current collector statistics
func (c *Collector) Stats() CollectorStats {
	return CollectorStats{
		MessagesProcessed: c.messagesProcessed.Load(),
		MessagesErrors:    c.messagesErrors.Load(),
		GPUsStored:        c.gpusStored.Load(),
		TelemetryStored:   c.telemetryStored.Load(),
	}
}

// CollectorStats holds collector statistics
type CollectorStats struct {
	MessagesProcessed int64
	MessagesErrors    int64
	GPUsStored        int64
	TelemetryStored   int64
}
