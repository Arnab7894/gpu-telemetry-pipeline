package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrRedisQueueClosed = errors.New("redis queue is closed")
	ErrRedisConnection  = errors.New("redis connection failed")
)

// RedisQueue implements MessageQueue interface using Redis
// Uses Redis Lists for message queuing and Sorted Sets for visibility timeout
type RedisQueue struct {
	client            *redis.Client
	logger            *slog.Logger
	visibilityTimeout time.Duration
	maxRetries        int
	consumerGroup     string
	consumerID        string

	subscriptions map[string]*redisSubscription
	subMu         sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Statistics
	totalPublished atomic.Int64
	totalDelivered atomic.Int64
	totalErrors    atomic.Int64

	closed atomic.Bool
}

type redisSubscription struct {
	topic    string
	handler  MessageHandler
	cancel   context.CancelFunc
	doneChan chan struct{}
}

// RedisQueueConfig configuration for Redis queue
type RedisQueueConfig struct {
	RedisURL          string
	ConsumerGroup     string
	ConsumerID        string
	VisibilityTimeout time.Duration
	MaxRetries        int
	PoolSize          int
}

// NewRedisQueue creates a new Redis-backed message queue
func NewRedisQueue(config RedisQueueConfig, logger *slog.Logger) (*RedisQueue, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if config.VisibilityTimeout == 0 {
		config.VisibilityTimeout = 5 * time.Minute
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	if config.PoolSize == 0 {
		config.PoolSize = 10
	}

	// Parse Redis URL
	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	opts.PoolSize = config.PoolSize

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRedisConnection, err)
	}

	queueCtx, queueCancel := context.WithCancel(context.Background())

	q := &RedisQueue{
		client:            client,
		logger:            logger.With("component", "redis_queue"),
		visibilityTimeout: config.VisibilityTimeout,
		maxRetries:        config.MaxRetries,
		consumerGroup:     config.ConsumerGroup,
		consumerID:        config.ConsumerID,
		subscriptions:     make(map[string]*redisSubscription),
		ctx:               queueCtx,
		cancel:            queueCancel,
	}

	logger.Info("Redis queue initialized",
		"redis_url", config.RedisURL,
		"consumer_group", config.ConsumerGroup,
		"consumer_id", config.ConsumerID,
		"visibility_timeout", config.VisibilityTimeout,
	)

	return q, nil
}

// Start begins processing messages
func (q *RedisQueue) Start(ctx context.Context) error {
	if q.closed.Load() {
		return ErrRedisQueueClosed
	}

	q.logger.Info("Starting Redis queue")

	// Start visibility timeout checker
	q.wg.Add(1)
	go q.visibilityTimeoutChecker()

	return nil
}

// Publish publishes a message to a topic
// Messages are pushed to Redis List: "queue:{topic}"
func (q *RedisQueue) Publish(ctx context.Context, msg *Message) error {
	if q.closed.Load() {
		return ErrRedisQueueClosed
	}

	// Ensure message has an ID
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Ensure timestamp is set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Push to Redis list (left push = FIFO when we RPOP)
	queueKey := fmt.Sprintf("queue:%s", msg.Topic)
	if err := q.client.LPush(ctx, queueKey, data).Err(); err != nil {
		q.totalErrors.Add(1)
		return fmt.Errorf("failed to push message to Redis: %w", err)
	}

	q.totalPublished.Add(1)
	q.logger.Debug("Message published",
		"topic", msg.Topic,
		"message_id", msg.ID,
		"queue_key", queueKey,
	)

	return nil
}

// Subscribe subscribes to a topic with a message handler
// Creates a consumer that polls messages from Redis
func (q *RedisQueue) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	if q.closed.Load() {
		return ErrRedisQueueClosed
	}

	q.subMu.Lock()
	if _, exists := q.subscriptions[topic]; exists {
		q.subMu.Unlock()
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}
	q.subMu.Unlock()

	subCtx, subCancel := context.WithCancel(q.ctx)

	sub := &redisSubscription{
		topic:    topic,
		handler:  handler,
		cancel:   subCancel,
		doneChan: make(chan struct{}),
	}

	q.subMu.Lock()
	q.subscriptions[topic] = sub
	q.subMu.Unlock()

	q.logger.Info("Subscribed to topic",
		"topic", topic,
		"consumer_group", q.consumerGroup,
		"consumer_id", q.consumerID,
	)

	// Start consumer loop
	q.wg.Add(1)
	go q.consumeLoop(subCtx, sub)

	return nil
}

// consumeLoop continuously polls for messages from Redis
func (q *RedisQueue) consumeLoop(ctx context.Context, sub *redisSubscription) {
	defer q.wg.Done()
	defer close(sub.doneChan)

	queueKey := fmt.Sprintf("queue:%s", sub.topic)
	processingKey := fmt.Sprintf("processing:%s:%s", sub.topic, q.consumerID)

	q.logger.Info("Starting consume loop",
		"topic", sub.topic,
		"queue_key", queueKey,
		"processing_key", processingKey,
	)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			q.logger.Info("Consume loop stopped", "topic", sub.topic)
			return
		case <-ticker.C:
			// Try to pop a message from the queue using BRPOPLPUSH for atomicity
			// This moves the message from queue to processing list
			result, err := q.client.BRPopLPush(ctx, queueKey, processingKey, time.Second).Result()
			if err != nil {
				if err == redis.Nil {
					// No messages available, continue polling
					continue
				}
				if errors.Is(err, context.Canceled) {
					return
				}
				q.logger.Error("Failed to pop message from Redis",
					"error", err,
					"topic", sub.topic,
				)
				q.totalErrors.Add(1)
				continue
			}

			// Deserialize message
			var msg Message
			if err := json.Unmarshal([]byte(result), &msg); err != nil {
				q.logger.Error("Failed to unmarshal message",
					"error", err,
					"topic", sub.topic,
				)
				// Remove invalid message from processing list
				q.client.LRem(ctx, processingKey, 1, result)
				q.totalErrors.Add(1)
				continue
			}

			// Add to visibility timeout sorted set
			visibilityKey := fmt.Sprintf("visibility:%s:%s", sub.topic, q.consumerGroup)
			visibilityUntil := time.Now().Add(q.visibilityTimeout).Unix()
			messageData := map[string]interface{}{
				"message_id":     msg.ID,
				"consumer_id":    q.consumerID,
				"delivery_count": 0,
				"message_json":   result,
			}
			metadataJSON, _ := json.Marshal(messageData)
			q.client.ZAdd(ctx, visibilityKey, redis.Z{
				Score:  float64(visibilityUntil),
				Member: string(metadataJSON),
			})

			q.totalDelivered.Add(1)

			// Handle message
			q.handleMessage(ctx, sub, &msg, processingKey, result)
		}
	}
}

// handleMessage processes a single message
func (q *RedisQueue) handleMessage(ctx context.Context, sub *redisSubscription, msg *Message, processingKey, rawMessage string) {
	// Call handler
	if err := sub.handler(ctx, msg); err != nil {
		q.logger.Error("Handler failed to process message",
			"error", err,
			"topic", sub.topic,
			"message_id", msg.ID,
		)
		q.totalErrors.Add(1)

		// Nack the message (will be requeued by visibility timeout checker)
		return
	}

	// Handler succeeded - ACK the message
	q.ack(ctx, sub.topic, msg.ID, processingKey, rawMessage)
}

// ack acknowledges a message (deletes it)
func (q *RedisQueue) ack(ctx context.Context, topic, messageID, processingKey, rawMessage string) error {
	// Remove from processing list
	if err := q.client.LRem(ctx, processingKey, 1, rawMessage).Err(); err != nil {
		q.logger.Error("Failed to remove message from processing list",
			"error", err,
			"message_id", messageID,
		)
		return err
	}

	// Remove from visibility set
	visibilityKey := fmt.Sprintf("visibility:%s:%s", topic, q.consumerGroup)
	// We need to remove by message_id pattern
	members, _ := q.client.ZRange(ctx, visibilityKey, 0, -1).Result()
	for _, member := range members {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(member), &data); err == nil {
			if data["message_id"] == messageID {
				q.client.ZRem(ctx, visibilityKey, member)
				break
			}
		}
	}

	q.logger.Debug("Message acknowledged",
		"topic", topic,
		"message_id", messageID,
	)

	return nil
}

// Ack is a no-op for Redis queue since we ack internally after successful handling
func (q *RedisQueue) Ack(topic, messageID string) error {
	// Already handled in ack() method
	return nil
}

// Nack negatively acknowledges a message (requeues it)
func (q *RedisQueue) Nack(topic, messageID string) error {
	// The visibility timeout checker will handle requeuing
	// We just need to remove from visibility set to make it eligible for immediate requeue
	ctx := context.Background()
	visibilityKey := fmt.Sprintf("visibility:%s:%s", topic, q.consumerGroup)

	members, _ := q.client.ZRange(ctx, visibilityKey, 0, -1).Result()
	for _, member := range members {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(member), &data); err == nil {
			if data["message_id"] == messageID {
				// Remove from visibility to trigger immediate requeue
				q.client.ZRem(ctx, visibilityKey, member)

				// Move back to queue
				queueKey := fmt.Sprintf("queue:%s", topic)
				if msgJSON, ok := data["message_json"].(string); ok {
					q.client.LPush(ctx, queueKey, msgJSON)
				}

				q.logger.Debug("Message nacked and requeued",
					"topic", topic,
					"message_id", messageID,
				)
				return nil
			}
		}
	}

	return fmt.Errorf("message not found in visibility set: %s", messageID)
}

// visibilityTimeoutChecker periodically checks for expired messages
func (q *RedisQueue) visibilityTimeoutChecker() {
	defer q.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	q.logger.Info("Visibility timeout checker started")

	for {
		select {
		case <-q.ctx.Done():
			q.logger.Info("Visibility timeout checker stopped")
			return
		case <-ticker.C:
			q.checkExpiredMessages()
		}
	}
}

// checkExpiredMessages finds and requeues messages past visibility timeout
func (q *RedisQueue) checkExpiredMessages() {
	ctx := context.Background()

	// Get all topics we're subscribed to
	q.subMu.RLock()
	topics := make([]string, 0, len(q.subscriptions))
	for topic := range q.subscriptions {
		topics = append(topics, topic)
	}
	q.subMu.RUnlock()

	now := time.Now().Unix()

	for _, topic := range topics {
		visibilityKey := fmt.Sprintf("visibility:%s:%s", topic, q.consumerGroup)
		queueKey := fmt.Sprintf("queue:%s", topic)

		// Get expired messages (score < now)
		expired, err := q.client.ZRangeByScore(ctx, visibilityKey, &redis.ZRangeBy{
			Min: "-inf",
			Max: fmt.Sprintf("%d", now),
		}).Result()

		if err != nil {
			q.logger.Error("Failed to get expired messages",
				"error", err,
				"topic", topic,
			)
			continue
		}

		for _, member := range expired {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(member), &data); err != nil {
				q.logger.Error("Failed to unmarshal visibility data", "error", err)
				q.client.ZRem(ctx, visibilityKey, member)
				continue
			}

			deliveryCount := int(data["delivery_count"].(float64))
			if deliveryCount >= q.maxRetries {
				// Move to dead letter queue
				q.logger.Warn("Message exceeded max retries",
					"message_id", data["message_id"],
					"topic", topic,
					"delivery_count", deliveryCount,
				)
				q.client.ZRem(ctx, visibilityKey, member)
				continue
			}

			// Requeue message
			if msgJSON, ok := data["message_json"].(string); ok {
				q.client.LPush(ctx, queueKey, msgJSON)
				q.logger.Debug("Requeued expired message",
					"message_id", data["message_id"],
					"topic", topic,
					"delivery_count", deliveryCount+1,
				)
			}

			// Remove from visibility set
			q.client.ZRem(ctx, visibilityKey, member)
		}
	}
}

// Unsubscribe removes a subscription
func (q *RedisQueue) Unsubscribe(topic string) error {
	q.subMu.Lock()
	sub, exists := q.subscriptions[topic]
	if !exists {
		q.subMu.Unlock()
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}

	delete(q.subscriptions, topic)
	q.subMu.Unlock()

	sub.cancel()
	<-sub.doneChan

	q.logger.Info("Unsubscribed from topic", "topic", topic)

	return nil
}

// Stop gracefully stops the queue
func (q *RedisQueue) Stop() error {
	if q.closed.Load() {
		return nil
	}

	q.logger.Info("Stopping Redis queue")

	q.cancel()
	q.wg.Wait()

	q.logger.Info("Redis queue stopped")

	return nil
}

// Close closes the Redis connection
func (q *RedisQueue) Close() error {
	if q.closed.Swap(true) {
		return nil
	}

	q.logger.Info("Closing Redis queue")

	// Stop all subscriptions
	q.subMu.Lock()
	for _, sub := range q.subscriptions {
		sub.cancel()
	}
	q.subMu.Unlock()

	// Wait for all goroutines
	q.wg.Wait()

	// Close Redis client
	if err := q.client.Close(); err != nil {
		q.logger.Error("Failed to close Redis client", "error", err)
		return err
	}

	q.logger.Info("Redis queue closed")

	return nil
}

// Stats returns queue statistics
func (q *RedisQueue) Stats() QueueStats {
	q.subMu.RLock()
	activeSubscribers := len(q.subscriptions)
	q.subMu.RUnlock()

	return QueueStats{
		TotalPublished:    q.totalPublished.Load(),
		TotalDelivered:    q.totalDelivered.Load(),
		TotalErrors:       q.totalErrors.Load(),
		ActiveSubscribers: activeSubscribers,
		QueueDepth:        0, // Would require querying all Redis queues
	}
}
