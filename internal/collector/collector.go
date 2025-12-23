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
	WorkerPoolSize = 50   // Number of concurrent workers
	WorkQueueSize  = 1000 // Buffer size for work queue
)

type workItem struct {
	ctx context.Context
	msg *mq.Message
}

// Collector subscribes to message queue and stores telemetry data
type Collector struct {
	config        *Config
	queue         mq.MessageQueue
	telemetryRepo storage.TelemetryRepository
	gpuRepo       storage.GPURepository
	logger        *slog.Logger

	// Worker pool
	workQueue   chan workItem
	workersDone chan struct{}

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
		workQueue:     make(chan workItem, WorkQueueSize),
		workersDone:   make(chan struct{}),
	}
}

// Start begins collecting telemetry messages
func (c *Collector) Start(ctx context.Context) error {
	c.logger.Info("Starting telemetry collector",
		"batch_size", c.config.BatchSize,
		"max_concurrent", c.config.MaxConcurrentHandlers,
		"worker_pool_size", WorkerPoolSize,
		"work_queue_size", WorkQueueSize,
	)

	// Start worker pool
	c.startWorkerPool(ctx)

	// Check from here.
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

	// Close work queue and wait for workers to finish
	close(c.workQueue)
	<-c.workersDone

	return ctx.Err()
}

// handleMessage enqueues message to worker pool for parallel processing
func (c *Collector) handleMessage(ctx context.Context, msg *mq.Message) error {
	// Enqueue work item (non-blocking with timeout)
	select {
	case c.workQueue <- workItem{ctx: ctx, msg: msg}:
		// Successfully enqueued
		return nil
	// Increase WorkQueueSize if seeing frequent drops or
	// Increase timeout if workers are slow but not failing
	case <-time.After(5 * time.Second):
		// Work queue is full, drop message
		c.messagesErrors.Add(1)
		c.logger.Warn("Work queue full, dropping message",
			"message_id", msg.ID,
			"queue_size", len(c.workQueue),
		)
		return nil // Return nil to ACK (don't block SSE stream)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// startWorkerPool starts a pool of workers to process messages in parallel
func (c *Collector) startWorkerPool(ctx context.Context) {
	c.logger.Info("Starting worker pool", "workers", WorkerPoolSize)

	for i := 0; i < WorkerPoolSize; i++ {
		go c.worker(ctx, i)
	}

	// Monitor workers completion
	go func() {
		<-ctx.Done()
		// Wait for work queue to be processed
		for len(c.workQueue) > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		close(c.workersDone)
	}()
}

// worker processes messages from the work queue
func (c *Collector) worker(ctx context.Context, workerID int) {
	logger := c.logger.With("worker_id", workerID)
	logger.Debug("Worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Worker stopped")
			return
		case item, ok := <-c.workQueue:
			if !ok {
				logger.Debug("Work queue closed, worker exiting")
				return
			}
			c.processMessage(item.ctx, item.msg)
		}
	}
}

// processMessage processes a single telemetry message
func (c *Collector) processMessage(ctx context.Context, msg *mq.Message) {
	c.messagesProcessed.Add(1)

	// Parse telemetry point from message payload
	var telemetry domain.TelemetryPoint
	if err := json.Unmarshal(msg.Payload, &telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Warn("Failed to unmarshal telemetry",
			"message_id", msg.ID,
			"error", err,
		)
		return
	}

	// Validate telemetry data
	if err := c.validateTelemetry(&telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Warn("Invalid telemetry data",
			"message_id", msg.ID,
			"gpu_uuid", telemetry.GPUUUID,
			"error", err,
		)
		return
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

	// Store telemetry point immediately
	if err := c.telemetryRepo.Store(&telemetry); err != nil {
		c.messagesErrors.Add(1)
		c.logger.Error("Failed to store telemetry",
			"message_id", msg.ID,
			"gpu_uuid", telemetry.GPUUUID,
			"metric", telemetry.MetricName,
			"batch_id", telemetry.BatchID,
			"error", err,
		)
		// Log error but don't requeue (already ACK'd)
		return
	}

	c.telemetryStored.Add(1)

	c.logger.Debug("Stored telemetry",
		"message_id", msg.ID,
		"gpu_uuid", telemetry.GPUUUID,
		"metric", telemetry.MetricName,
		"batch_id", telemetry.BatchID,
		"value", telemetry.Value,
	)
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
	// Extract ALL GPU fields from message metadata
	hostname, _ := msg.Metadata["hostname"].(string)
	device, _ := msg.Metadata["device"].(string)
	gpuIndex, _ := msg.Metadata["gpu_index"].(string)
	modelName, _ := msg.Metadata["model_name"].(string)
	container, _ := msg.Metadata["container"].(string)
	pod, _ := msg.Metadata["pod"].(string)
	namespace, _ := msg.Metadata["namespace"].(string)

	// Always create GPU with all available fields
	return &domain.GPU{
		UUID:      telemetry.GPUUUID,
		DeviceID:  device,
		GPUIndex:  gpuIndex,
		ModelName: modelName,
		Hostname:  hostname,
		Container: container,
		Pod:       pod,
		Namespace: namespace,
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
