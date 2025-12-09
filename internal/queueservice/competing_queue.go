package queueservice

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
)

var (
	ErrTopicNotFound         = errors.New("topic not found")
	ErrConsumerGroupNotFound = errors.New("consumer group not found")
	ErrMessageNotFound       = errors.New("message not found")
	ErrNoConsumersAvailable  = errors.New("no consumers available")
)

// CompetingConsumerQueue implements a queue with competing consumer pattern
type CompetingConsumerQueue struct {
	topics             map[string]*Topic
	mu                 sync.RWMutex
	logger             *slog.Logger
	visibilityTimeout  time.Duration
	maxRetries         int
	shutdownChan       chan struct{}
	deadLetterMessages map[string]*Message
	deadLetterMu       sync.RWMutex
}

// Topic represents a message topic
type Topic struct {
	name           string
	messages       chan *Message
	consumerGroups map[string]*ConsumerGroup
	mu             sync.RWMutex
}

// ConsumerGroup manages competing consumers
type ConsumerGroup struct {
	name      string
	consumers map[string]*Consumer
	pending   *PendingList
	mu        sync.RWMutex
	topic     *Topic
}

// Consumer represents a single consumer in a group
type Consumer struct {
	ID                 string
	MessageChan        chan *Message
	LastActivity       time.Time
	activeMessageCount int64 // atomic counter for messages being processed
	mu                 sync.RWMutex
}

// GetActiveMessageCount returns the current count of active messages
func (c *Consumer) GetActiveMessageCount() int64 {
	return atomic.LoadInt64(&c.activeMessageCount)
}

// PendingList tracks messages being processed
type PendingList struct {
	entries map[string]*PendingEntry
	mu      sync.RWMutex
}

// Config for the queue
type Config struct {
	BufferSize        int
	VisibilityTimeout time.Duration
	MaxRetries        int
}

// NewCompetingConsumerQueue creates a new queue instance
func NewCompetingConsumerQueue(config Config, logger *slog.Logger) *CompetingConsumerQueue {
	if logger == nil {
		logger = slog.Default()
	}

	if config.VisibilityTimeout == 0 {
		config.VisibilityTimeout = 5 * time.Minute
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}

	return &CompetingConsumerQueue{
		topics:             make(map[string]*Topic),
		logger:             logger.With("component", "queue"),
		visibilityTimeout:  config.VisibilityTimeout,
		maxRetries:         config.MaxRetries,
		shutdownChan:       make(chan struct{}),
		deadLetterMessages: make(map[string]*Message),
	}
}

// Start begins the queue background processes
func (q *CompetingConsumerQueue) Start(ctx context.Context) error {
	q.logger.Info("Starting competing consumer queue",
		"visibility_timeout", q.visibilityTimeout,
		"max_retries", q.maxRetries,
	)

	// Start visibility timeout checker
	go q.visibilityTimeoutChecker(ctx)

	return nil
}

// Publish adds a message to a topic
func (q *CompetingConsumerQueue) Publish(ctx context.Context, topicName string, payload json.RawMessage, metadata map[string]interface{}) (*Message, error) {
	topic := q.getOrCreateTopic(topicName)

	msg := &Message{
		ID:            uuid.New().String(),
		Topic:         topicName,
		Payload:       payload,
		Headers:       make(map[string]string),
		Metadata:      metadata,
		Timestamp:     time.Now(),
		DeliveryCount: 0,
	}

	select {
	case topic.messages <- msg:
		q.logger.Debug("Message published",
			"topic", topicName,
			"message_id", msg.ID,
		)
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, errors.New("topic buffer full")
	}
}

// CreateConsumerGroup creates or gets a consumer group for a topic
func (q *CompetingConsumerQueue) CreateConsumerGroup(topicName, groupName string) error {
	topic := q.getOrCreateTopic(topicName)

	topic.mu.Lock()
	defer topic.mu.Unlock()

	if _, exists := topic.consumerGroups[groupName]; exists {
		return nil // Already exists
	}

	topic.consumerGroups[groupName] = &ConsumerGroup{
		name:      groupName,
		consumers: make(map[string]*Consumer),
		pending:   NewPendingList(),
		topic:     topic,
	}

	q.logger.Info("Consumer group created",
		"topic", topicName,
		"group", groupName,
	)

	return nil
}

// RegisterConsumer adds a consumer to a consumer group
func (q *CompetingConsumerQueue) RegisterConsumer(topicName, groupName, consumerID string) (*Consumer, error) {
	q.mu.RLock()
	topic, exists := q.topics[topicName]
	q.mu.RUnlock()

	if !exists {
		return nil, ErrTopicNotFound
	}

	topic.mu.RLock()
	group, exists := topic.consumerGroups[groupName]
	topic.mu.RUnlock()

	if !exists {
		return nil, ErrConsumerGroupNotFound
	}

	group.mu.Lock()
	defer group.mu.Unlock()

	// Check if consumer already exists
	if consumer, exists := group.consumers[consumerID]; exists {
		consumer.mu.Lock()
		consumer.LastActivity = time.Now()
		consumer.mu.Unlock()
		return consumer, nil
	}

	// Create new consumer
	consumer := &Consumer{
		ID:           consumerID,
		MessageChan:  make(chan *Message, 10),
		LastActivity: time.Now(),
	}

	group.consumers[consumerID] = consumer

	q.logger.Info("Consumer registered",
		"topic", topicName,
		"group", groupName,
		"consumer_id", consumerID,
	)

	// Start delivering messages to this consumer group
	go q.deliverToConsumerGroup(context.Background(), topic, group)

	return consumer, nil
}

// UnregisterConsumer removes a consumer from a group
func (q *CompetingConsumerQueue) UnregisterConsumer(topicName, groupName, consumerID string) error {
	q.mu.RLock()
	topic, exists := q.topics[topicName]
	q.mu.RUnlock()

	if !exists {
		return ErrTopicNotFound
	}

	topic.mu.RLock()
	group, exists := topic.consumerGroups[groupName]
	topic.mu.RUnlock()

	if !exists {
		return ErrConsumerGroupNotFound
	}

	group.mu.Lock()
	consumer, exists := group.consumers[consumerID]
	if exists {
		close(consumer.MessageChan)
		delete(group.consumers, consumerID)
	}
	group.mu.Unlock()

	if !exists {
		return errors.New("consumer not found")
	}

	q.logger.Info("Consumer unregistered",
		"topic", topicName,
		"group", groupName,
		"consumer_id", consumerID,
	)

	return nil
}

// Ack acknowledges a message (removes from pending list)
func (q *CompetingConsumerQueue) Ack(topicName, groupName, messageID string) error {
	q.mu.RLock()
	topic, exists := q.topics[topicName]
	q.mu.RUnlock()

	if !exists {
		return ErrTopicNotFound
	}

	topic.mu.RLock()
	group, exists := topic.consumerGroups[groupName]
	topic.mu.RUnlock()

	if !exists {
		return ErrConsumerGroupNotFound
	}

	// Get the entry first to find which consumer processed it
	entry := group.pending.Get(messageID)
	if entry == nil {
		return ErrMessageNotFound
	}

	removed := group.pending.Remove(messageID)
	if !removed {
		return ErrMessageNotFound
	}

	// Decrement the consumer's active message count
	group.mu.RLock()
	consumer, exists := group.consumers[entry.ConsumerID]
	group.mu.RUnlock()
	if exists {
		atomic.AddInt64(&consumer.activeMessageCount, -1)
	}

	q.logger.Debug("Message acknowledged",
		"topic", topicName,
		"group", groupName,
		"message_id", messageID,
	)

	return nil
}

// Nack negatively acknowledges a message (requeue immediately)
func (q *CompetingConsumerQueue) Nack(topicName, groupName, messageID string) error {
	q.mu.RLock()
	topic, exists := q.topics[topicName]
	q.mu.RUnlock()

	if !exists {
		return ErrTopicNotFound
	}

	topic.mu.RLock()
	group, exists := topic.consumerGroups[groupName]
	topic.mu.RUnlock()

	if !exists {
		return ErrConsumerGroupNotFound
	}

	entry := group.pending.Get(messageID)
	if entry == nil {
		return ErrMessageNotFound
	}

	// Remove from pending
	group.pending.Remove(messageID)

	// Decrement the consumer's active message count
	group.mu.RLock()
	consumer, exists := group.consumers[entry.ConsumerID]
	group.mu.RUnlock()
	if exists {
		atomic.AddInt64(&consumer.activeMessageCount, -1)
	}

	// Increment delivery count
	entry.Message.DeliveryCount++

	// Requeue if under max retries
	if entry.Message.DeliveryCount < q.maxRetries {
		select {
		case topic.messages <- entry.Message:
			q.logger.Debug("Message requeued",
				"topic", topicName,
				"message_id", messageID,
				"delivery_count", entry.Message.DeliveryCount,
			)
		default:
			q.logger.Warn("Failed to requeue message - buffer full",
				"topic", topicName,
				"message_id", messageID,
			)
		}
	} else {
		// Move to dead letter queue
		q.deadLetterMu.Lock()
		q.deadLetterMessages[messageID] = entry.Message
		q.deadLetterMu.Unlock()

		q.logger.Warn("Message moved to dead letter queue",
			"topic", topicName,
			"message_id", messageID,
			"delivery_count", entry.Message.DeliveryCount,
		)
	}

	return nil
}

// deliverToConsumerGroup continuously delivers messages to consumers in a group
func (q *CompetingConsumerQueue) deliverToConsumerGroup(ctx context.Context, topic *Topic, group *ConsumerGroup) {
	for {
		select {
		case msg := <-topic.messages:
			// Select a consumer (round-robin/least-loaded)
			consumer := q.selectConsumer(group)
			if consumer == nil {
				// No consumers available, put message back
				select {
				case topic.messages <- msg:
				default:
					q.logger.Warn("No consumers and buffer full, message lost",
						"topic", topic.name,
						"group", group.name,
						"message_id", msg.ID,
					)
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Add to pending list
			entry := &PendingEntry{
				Message:         msg,
				ConsumerID:      consumer.ID,
				ConsumerGroup:   group.name,
				DeliveryTime:    time.Now(),
				VisibilityUntil: time.Now().Add(q.visibilityTimeout),
				DeliveryCount:   msg.DeliveryCount + 1,
			}
			group.pending.Add(msg.ID, entry)

			// Increment consumer's active message count
			atomic.AddInt64(&consumer.activeMessageCount, 1)

			// Deliver to consumer (non-blocking)
			select {
			case consumer.MessageChan <- msg:
				q.logger.Debug("Message delivered to consumer",
					"topic", topic.name,
					"group", group.name,
					"consumer_id", consumer.ID,
					"message_id", msg.ID,
					"active_count", atomic.LoadInt64(&consumer.activeMessageCount),
				)
			default:
				// Consumer buffer full, remove from pending and requeue
				group.pending.Remove(msg.ID)
				// Decrement count since delivery failed
				atomic.AddInt64(&consumer.activeMessageCount, -1)
				select {
				case topic.messages <- msg:
				default:
					q.logger.Warn("Consumer busy and buffer full, message lost",
						"topic", topic.name,
						"message_id", msg.ID,
					)
				}
			}

		case <-ctx.Done():
			return
		case <-q.shutdownChan:
			return
		}
	}
}

// selectConsumer selects the best consumer to deliver to using least-loaded strategy
func (q *CompetingConsumerQueue) selectConsumer(group *ConsumerGroup) *Consumer {
	group.mu.RLock()
	defer group.mu.RUnlock()

	if len(group.consumers) == 0 {
		return nil
	}

	// Least-loaded selection: pick consumer with fewest active messages
	var selected *Consumer
	minLoad := int64(-1)

	for _, consumer := range group.consumers {
		load := atomic.LoadInt64(&consumer.activeMessageCount)
		if minLoad == -1 || load < minLoad {
			minLoad = load
			selected = consumer
		}
	}

	return selected
}

// visibilityTimeoutChecker checks for expired messages and requeues them
func (q *CompetingConsumerQueue) visibilityTimeoutChecker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.checkExpiredMessages()
		case <-ctx.Done():
			return
		case <-q.shutdownChan:
			return
		}
	}
}

// checkExpiredMessages finds and requeues messages past visibility timeout
func (q *CompetingConsumerQueue) checkExpiredMessages() {
	q.mu.RLock()
	topics := make([]*Topic, 0, len(q.topics))
	for _, topic := range q.topics {
		topics = append(topics, topic)
	}
	q.mu.RUnlock()

	now := time.Now()
	for _, topic := range topics {
		topic.mu.RLock()
		groups := make([]*ConsumerGroup, 0, len(topic.consumerGroups))
		for _, group := range topic.consumerGroups {
			groups = append(groups, group)
		}
		topic.mu.RUnlock()

		for _, group := range groups {
			expired := group.pending.GetExpired(now)

			for _, entry := range expired {
				// Remove from pending
				group.pending.Remove(entry.Message.ID)

				// Increment delivery count
				entry.Message.DeliveryCount++

				// Requeue or dead letter
				if entry.Message.DeliveryCount < q.maxRetries {
					select {
					case topic.messages <- entry.Message:
						q.logger.Info("Message requeued after visibility timeout",
							"topic", topic.name,
							"message_id", entry.Message.ID,
							"delivery_count", entry.Message.DeliveryCount,
						)
					default:
						q.logger.Warn("Failed to requeue expired message - buffer full",
							"topic", topic.name,
							"message_id", entry.Message.ID,
						)
					}
				} else {
					q.deadLetterMu.Lock()
					q.deadLetterMessages[entry.Message.ID] = entry.Message
					q.deadLetterMu.Unlock()

					q.logger.Warn("Expired message moved to dead letter queue",
						"topic", topic.name,
						"message_id", entry.Message.ID,
						"delivery_count", entry.Message.DeliveryCount,
					)
				}
			}
		}
	}
}

// getOrCreateTopic gets or creates a topic
func (q *CompetingConsumerQueue) getOrCreateTopic(name string) *Topic {
	q.mu.Lock()
	defer q.mu.Unlock()

	topic, exists := q.topics[name]
	if !exists {
		topic = &Topic{
			name:           name,
			messages:       make(chan *Message, 1000),
			consumerGroups: make(map[string]*ConsumerGroup),
		}
		q.topics[name] = topic
		q.logger.Info("Topic created", "topic", name)
	}

	return topic
}

// Stats returns queue statistics
func (q *CompetingConsumerQueue) Stats() map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := make(map[string]interface{})
	topics := make([]map[string]interface{}, 0, len(q.topics))

	for name, topic := range q.topics {
		topic.mu.RLock()

		groups := make([]map[string]interface{}, 0, len(topic.consumerGroups))
		for groupName, group := range topic.consumerGroups {
			group.mu.RLock()
			groupStats := map[string]interface{}{
				"name":             groupName,
				"consumer_count":   len(group.consumers),
				"pending_messages": group.pending.Count(),
			}
			group.mu.RUnlock()
			groups = append(groups, groupStats)
		}

		topicStats := map[string]interface{}{
			"name":            name,
			"queue_depth":     len(topic.messages),
			"consumer_groups": groups,
		}
		topics = append(topics, topicStats)

		topic.mu.RUnlock()
	}

	q.deadLetterMu.RLock()
	deadLetterCount := len(q.deadLetterMessages)
	q.deadLetterMu.RUnlock()

	stats["topics"] = topics
	stats["dead_letter_count"] = deadLetterCount

	return stats
}

// Shutdown gracefully shuts down the queue
func (q *CompetingConsumerQueue) Shutdown(ctx context.Context) error {
	q.logger.Info("Shutting down queue")
	close(q.shutdownChan)

	// Close all consumer channels
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, topic := range q.topics {
		topic.mu.RLock()
		for _, group := range topic.consumerGroups {
			group.mu.RLock()
			for _, consumer := range group.consumers {
				close(consumer.MessageChan)
			}
			group.mu.RUnlock()
		}
		topic.mu.RUnlock()
	}

	return nil
}

// NewPendingList creates a new pending list
func NewPendingList() *PendingList {
	return &PendingList{
		entries: make(map[string]*PendingEntry),
	}
}

// Add adds an entry to the pending list
func (pl *PendingList) Add(messageID string, entry *PendingEntry) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.entries[messageID] = entry
}

// Remove removes an entry from the pending list
func (pl *PendingList) Remove(messageID string) bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if _, exists := pl.entries[messageID]; exists {
		delete(pl.entries, messageID)
		return true
	}
	return false
}

// Get retrieves an entry from the pending list
func (pl *PendingList) Get(messageID string) *PendingEntry {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.entries[messageID]
}

// GetExpired returns all entries past their visibility timeout
func (pl *PendingList) GetExpired(now time.Time) []*PendingEntry {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	expired := make([]*PendingEntry, 0)
	for _, entry := range pl.entries {
		if now.After(entry.VisibilityUntil) {
			expired = append(expired, entry)
		}
	}

	return expired
}

// Count returns the number of pending messages
func (pl *PendingList) Count() int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return len(pl.entries)
}

// GetConsumerID returns the consumer ID for a message
func (pl *PendingList) GetConsumerID(messageID string) string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	if entry, exists := pl.entries[messageID]; exists {
		return entry.ConsumerID
	}
	return ""
}

// String returns a debug string representation
func (q *CompetingConsumerQueue) String() string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return fmt.Sprintf("Queue{topics=%d, visibility_timeout=%v, max_retries=%d}",
		len(q.topics), q.visibilityTimeout, q.maxRetries)
}
