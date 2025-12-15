package queueservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/redis/go-redis/v9"
)

// RedisQueue implements a competing consumer queue backed by Redis
// Messages are stored in Redis Lists, and PEL (Pending Entry List) uses Sorted Sets
type RedisQueue struct {
	client            *redis.Client
	logger            *slog.Logger
	visibilityTimeout time.Duration
	maxRetries        int
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup

	// Consumer groups: topic -> consumer_group -> consumers
	consumerGroups map[string]map[string]*QueueConsumerGroup
	mu             sync.RWMutex

	// Dead letter queue
	deadLetterQueue []*PendingEntry
	dlqMu           sync.Mutex
}

// QueueConsumerGroup represents a group of consumers for a topic
type QueueConsumerGroup struct {
	Name      string
	Consumers map[string]*QueueConsumer
	mu        sync.RWMutex
}

// QueueConsumer represents a single consumer in a consumer group
type QueueConsumer struct {
	ID       string
	EventsCh chan *mq.Message
}

// PendingEntry represents a message in the Pending Entry List (PEL)
type PendingEntry struct {
	Message       *mq.Message
	ConsumerGroup string
	ConsumerID    string
	VisibleAt     time.Time
	DeliveryCount int
}

// NewRedisQueue creates a new Redis-backed queue
func NewRedisQueue(redisURL string, visibilityTimeout time.Duration, maxRetries int, logger *slog.Logger) (*RedisQueue, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	queueCtx, queueCancel := context.WithCancel(context.Background())

	q := &RedisQueue{
		client:            client,
		logger:            logger.With("component", "redis_queue"),
		visibilityTimeout: visibilityTimeout,
		maxRetries:        maxRetries,
		ctx:               queueCtx,
		cancel:            queueCancel,
		consumerGroups:    make(map[string]map[string]*QueueConsumerGroup),
		deadLetterQueue:   make([]*PendingEntry, 0),
	}

	// Start visibility timeout checker
	q.wg.Add(1)
	go q.visibilityTimeoutChecker()

	logger.Info("Redis queue initialized",
		"redis_url", redisURL,
		"visibility_timeout", visibilityTimeout,
		"max_retries", maxRetries,
	)

	return q, nil
}

// Publish publishes a message to a topic (stores in Redis List)
func (q *RedisQueue) Publish(ctx context.Context, msg *mq.Message) error {
	queueKey := fmt.Sprintf("queue:%s", msg.Topic)

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// LPUSH adds to the head of the list (FIFO when using RPOP)
	if err := q.client.LPush(ctx, queueKey, msgJSON).Err(); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	q.logger.Debug("Message published",
		"topic", msg.Topic,
		"message_id", msg.ID,
	)

	// TODO: Notify all consumers in all groups for this topic
	q.notifyConsumers(msg.Topic)

	return nil
}

// FetchBatch fetches a batch of messages for a consumer (prefetch/batch delivery)
// This implements RabbitMQ-style prefetch where consumer gets N messages at once
func (q *RedisQueue) FetchBatch(ctx context.Context, topic, consumerGroup, consumerID string, count int) ([]*mq.Message, error) {
	messages := make([]*mq.Message, 0, count)

	q.logger.Debug("Fetching batch",
		"topic", topic,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
		"requested_count", count,
	)

	// Fetch messages up to count
	for i := 0; i < count; i++ {
		// Pop message from queue with timeout
		msg, err := q.popMessage(topic)
		if err != nil {
			// No more messages available
			break
		}

		// Add to PEL (Pending Entry List)
		if err := q.addToPEL(msg, consumerGroup, consumerID); err != nil {
			q.logger.Error("Failed to add message to PEL",
				"error", err,
				"message_id", msg.ID,
			)
			// Requeue the message
			_ = q.requeueMessage(msg)
			continue
		}

		messages = append(messages, msg)
	}

	q.logger.Info("Batch fetched",
		"topic", topic,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
		"fetched_count", len(messages),
		"requested_count", count,
	)

	return messages, nil
}

// RegisterConsumer registers a consumer in a consumer group for a topic
func (q *RedisQueue) RegisterConsumer(topic, consumerGroup, consumerID string) chan *mq.Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Initialize topic map
	if q.consumerGroups[topic] == nil {
		q.consumerGroups[topic] = make(map[string]*QueueConsumerGroup)
	}

	// Initialize consumer group
	if q.consumerGroups[topic][consumerGroup] == nil {
		q.consumerGroups[topic][consumerGroup] = &QueueConsumerGroup{
			Name:      consumerGroup,
			Consumers: make(map[string]*QueueConsumer),
		}
	}

	group := q.consumerGroups[topic][consumerGroup]
	group.mu.Lock()
	defer group.mu.Unlock()

	// Create consumer with buffered channel
	consumer := &QueueConsumer{
		ID:       consumerID,
		EventsCh: make(chan *mq.Message, 100),
	}

	group.Consumers[consumerID] = consumer

	q.logger.Info("Consumer registered",
		"topic", topic,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
	)

	// Start delivery goroutine for this consumer GROUP (only if not already started)
	// This ensures fair distribution and prevents duplicate consumption
	if len(group.Consumers) == 1 {
		// First consumer in this group - start the shared delivery goroutine
		q.wg.Add(1)
		go q.deliverMessagesToGroup(topic, consumerGroup)
		q.logger.Info("Started shared delivery goroutine for consumer group",
			"topic", topic,
			"consumer_group", consumerGroup,
		)
	}

	return consumer.EventsCh
}

// UnregisterConsumer removes a consumer from a consumer group
func (q *RedisQueue) UnregisterConsumer(topic, consumerGroup, consumerID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.consumerGroups[topic] == nil {
		return
	}

	group := q.consumerGroups[topic][consumerGroup]
	if group == nil {
		return
	}

	group.mu.Lock()
	defer group.mu.Unlock()

	if consumer, exists := group.Consumers[consumerID]; exists {
		close(consumer.EventsCh)
		delete(group.Consumers, consumerID)

		q.logger.Info("Consumer unregistered",
			"topic", topic,
			"consumer_group", consumerGroup,
			"consumer_id", consumerID,
		)
	}
}

// deliverMessagesToGroup continuously delivers messages to consumers in a group (round-robin)
func (q *RedisQueue) deliverMessagesToGroup(topic, consumerGroup string) {
	defer q.wg.Done()

	consumerIndex := 0
	batchSize := 50 // Process multiple messages before checking consumer list

	for {
		select {
		case <-q.ctx.Done():
			return
		default:
			// Get consumer list once per batch to reduce lock contention
			q.mu.RLock()
			if q.consumerGroups[topic] == nil || q.consumerGroups[topic][consumerGroup] == nil {
				q.mu.RUnlock()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			group := q.consumerGroups[topic][consumerGroup]
			group.mu.RLock()

			if len(group.Consumers) == 0 {
				group.mu.RUnlock()
				q.mu.RUnlock()
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Get consumer IDs for round-robin (cached for batch)
			consumerIDs := make([]string, 0, len(group.Consumers))
			for id := range group.Consumers {
				consumerIDs = append(consumerIDs, id)
			}

			group.mu.RUnlock()
			q.mu.RUnlock()

			// Process batch of messages with cached consumer list
			for i := 0; i < batchSize; i++ {
				// Try to pop a message from Redis
				msg, err := q.popMessage(topic)
				if err != nil || msg == nil {
					// No more messages available
					time.Sleep(10 * time.Millisecond)
					break
				}

				// Round-robin: pick consumer
				selectedConsumerID := consumerIDs[consumerIndex%len(consumerIDs)]
				consumerIndex++

				// Add to PEL for the selected consumer
				if err := q.addToPEL(msg, consumerGroup, selectedConsumerID); err != nil {
					q.logger.Error("Failed to add message to PEL",
						"error", err,
						"message_id", msg.ID,
					)
					// Put message back in queue
					q.requeueMessage(msg)
					continue
				}

				// Deliver to the selected consumer
				q.deliverToConsumer(topic, consumerGroup, selectedConsumerID, msg)
			}
		}
	}
}

// popMessage pops a message from the Redis queue (RPOP for FIFO)
func (q *RedisQueue) popMessage(topic string) (*mq.Message, error) {
	queueKey := fmt.Sprintf("queue:%s", topic)

	// BRPOP with 1 second timeout
	result, err := q.client.BRPop(q.ctx, 1*time.Second, queueKey).Result()
	if err == redis.Nil {
		return nil, nil // No message available
	}
	if err != nil {
		return nil, fmt.Errorf("failed to pop message: %w", err)
	}

	// result[0] is the key, result[1] is the value
	if len(result) < 2 {
		return nil, nil
	}

	var msg mq.Message
	if err := json.Unmarshal([]byte(result[1]), &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// addToPEL adds a message to the Pending Entry List (Sorted Set in Redis)
func (q *RedisQueue) addToPEL(msg *mq.Message, consumerGroup, consumerID string) error {
	pelKey := fmt.Sprintf("pel:%s:%s", msg.Topic, consumerGroup)
	visibleAt := time.Now().Add(q.visibilityTimeout)

	entry := PendingEntry{
		Message:       msg,
		ConsumerGroup: consumerGroup,
		ConsumerID:    consumerID,
		VisibleAt:     visibleAt,
		DeliveryCount: 1,
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal PEL entry: %w", err)
	}

	// Use sorted set with visibility timestamp as score
	score := float64(visibleAt.Unix())
	if err := q.client.ZAdd(q.ctx, pelKey, redis.Z{
		Score:  score,
		Member: entryJSON,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add to PEL: %w", err)
	}

	q.logger.Debug("Message added to PEL",
		"message_id", msg.ID,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
		"visible_at", visibleAt,
	)

	return nil
}

// deliverToConsumer sends a message to a specific consumer's channel
func (q *RedisQueue) deliverToConsumer(topic, consumerGroup, consumerID string, msg *mq.Message) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.consumerGroups[topic] == nil {
		q.logger.Warn("No consumer groups for topic", "topic", topic)
		return
	}

	group := q.consumerGroups[topic][consumerGroup]
	if group == nil {
		q.logger.Warn("Consumer group not found",
			"topic", topic,
			"consumer_group", consumerGroup,
		)
		return
	}

	group.mu.RLock()
	defer group.mu.RUnlock()

	consumer := group.Consumers[consumerID]
	if consumer == nil {
		q.logger.Warn("Consumer not found",
			"consumer_id", consumerID,
		)
		return
	}

	// Non-blocking send
	select {
	case consumer.EventsCh <- msg:
		q.logger.Debug("Message delivered to consumer",
			"message_id", msg.ID,
			"consumer_id", consumerID,
		)
	default:
		q.logger.Warn("Consumer channel full, message dropped",
			"consumer_id", consumerID,
			"message_id", msg.ID,
		)
	}
}

// notifyConsumers signals that new messages are available (by triggering delivery loops)
func (q *RedisQueue) notifyConsumers(topic string) {
	// The deliverMessages goroutines will automatically pick up new messages
	// No explicit notification needed with the polling approach
}

// Ack acknowledges a message (removes from PEL)
func (q *RedisQueue) Ack(topic, consumerGroup, messageID string) error {
	pelKey := fmt.Sprintf("pel:%s:%s", topic, consumerGroup)

	// Find and remove the entry from PEL
	entries, err := q.client.ZRange(q.ctx, pelKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to get PEL entries: %w", err)
	}

	for _, entryJSON := range entries {
		var entry PendingEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			continue
		}

		if entry.Message.ID == messageID {
			// Remove from PEL
			if err := q.client.ZRem(q.ctx, pelKey, entryJSON).Err(); err != nil {
				return fmt.Errorf("failed to remove from PEL: %w", err)
			}

			q.logger.Debug("Message acknowledged",
				"message_id", messageID,
				"consumer_group", consumerGroup,
			)

			return nil
		}
	}

	return fmt.Errorf("message not found in PEL: %s", messageID)
}

// Nack negatively acknowledges a message (requeues immediately)
func (q *RedisQueue) Nack(topic, consumerGroup, messageID string) error {
	pelKey := fmt.Sprintf("pel:%s:%s", topic, consumerGroup)

	// Find the entry in PEL
	entries, err := q.client.ZRange(q.ctx, pelKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to get PEL entries: %w", err)
	}

	for _, entryJSON := range entries {
		var entry PendingEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			continue
		}

		if entry.Message.ID == messageID {
			// Remove from PEL
			if err := q.client.ZRem(q.ctx, pelKey, entryJSON).Err(); err != nil {
				return fmt.Errorf("failed to remove from PEL: %w", err)
			}

			// Requeue message
			if err := q.requeueMessage(entry.Message); err != nil {
				return fmt.Errorf("failed to requeue message: %w", err)
			}

			q.logger.Debug("Message nacked and requeued",
				"message_id", messageID,
				"consumer_group", consumerGroup,
			)

			return nil
		}
	}

	return fmt.Errorf("message not found in PEL: %s", messageID)
}

// requeueMessage puts a message back in the queue
func (q *RedisQueue) requeueMessage(msg *mq.Message) error {
	queueKey := fmt.Sprintf("queue:%s", msg.Topic)

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// RPUSH adds to the end of the list (so it's processed later)
	if err := q.client.RPush(q.ctx, queueKey, msgJSON).Err(); err != nil {
		return fmt.Errorf("failed to requeue message: %w", err)
	}

	return nil
}

// visibilityTimeoutChecker periodically checks for expired messages in PEL
func (q *RedisQueue) visibilityTimeoutChecker() {
	defer q.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-ticker.C:
			q.checkExpiredMessages()
		}
	}
}

// checkExpiredMessages finds and requeues expired messages in all PELs
func (q *RedisQueue) checkExpiredMessages() {
	// Get all PEL keys
	pattern := "pel:*"
	keys, err := q.client.Keys(q.ctx, pattern).Result()
	if err != nil {
		q.logger.Error("Failed to get PEL keys", "error", err)
		return
	}

	now := time.Now()

	for _, pelKey := range keys {
		// Get expired entries (score < now)
		entries, err := q.client.ZRangeByScore(q.ctx, pelKey, &redis.ZRangeBy{
			Min: "-inf",
			Max: fmt.Sprintf("%d", now.Unix()),
		}).Result()

		if err != nil {
			q.logger.Error("Failed to get expired entries",
				"pel_key", pelKey,
				"error", err,
			)
			continue
		}

		for _, entryJSON := range entries {
			var entry PendingEntry
			if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
				q.logger.Error("Failed to unmarshal PEL entry", "error", err)
				continue
			}

			// Check if max retries exceeded
			if entry.DeliveryCount >= q.maxRetries {
				// Move to dead letter queue
				q.dlqMu.Lock()
				q.deadLetterQueue = append(q.deadLetterQueue, &entry)
				q.dlqMu.Unlock()

				// Remove from PEL
				q.client.ZRem(q.ctx, pelKey, entryJSON)

				q.logger.Warn("Message moved to dead letter queue",
					"message_id", entry.Message.ID,
					"delivery_count", entry.DeliveryCount,
				)
			} else {
				// Increment delivery count and requeue
				entry.DeliveryCount++

				// Remove old entry from PEL
				q.client.ZRem(q.ctx, pelKey, entryJSON)

				// Requeue message
				if err := q.requeueMessage(entry.Message); err != nil {
					q.logger.Error("Failed to requeue expired message",
						"message_id", entry.Message.ID,
						"error", err,
					)
				} else {
					q.logger.Info("Message requeued due to visibility timeout",
						"message_id", entry.Message.ID,
						"delivery_count", entry.DeliveryCount,
					)
				}
			}
		}
	}
}

// GetStats returns queue statistics
func (q *RedisQueue) GetStats() map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := make(map[string]interface{})
	topics := make([]map[string]interface{}, 0)

	for topic, groups := range q.consumerGroups {
		// Get queue depth from Redis
		queueKey := fmt.Sprintf("queue:%s", topic)
		queueDepth, _ := q.client.LLen(q.ctx, queueKey).Result()

		consumerGroups := make([]map[string]interface{}, 0)
		for groupName, group := range groups {
			group.mu.RLock()
			consumerCount := len(group.Consumers)
			group.mu.RUnlock()

			// Get pending messages count from Redis
			pelKey := fmt.Sprintf("pel:%s:%s", topic, groupName)
			pendingCount, _ := q.client.ZCard(q.ctx, pelKey).Result()

			consumerGroups = append(consumerGroups, map[string]interface{}{
				"name":             groupName,
				"consumer_count":   consumerCount,
				"pending_messages": pendingCount,
			})
		}

		topics = append(topics, map[string]interface{}{
			"name":            topic,
			"queue_depth":     queueDepth,
			"consumer_groups": consumerGroups,
		})
	}

	q.dlqMu.Lock()
	dlqCount := len(q.deadLetterQueue)
	q.dlqMu.Unlock()

	stats["topics"] = topics
	stats["dead_letter_count"] = dlqCount

	return stats
}

// Close closes the queue and cleans up resources
func (q *RedisQueue) Close() error {
	q.cancel()
	q.wg.Wait()

	if err := q.client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis client: %w", err)
	}

	q.logger.Info("Redis queue closed")
	return nil
}
