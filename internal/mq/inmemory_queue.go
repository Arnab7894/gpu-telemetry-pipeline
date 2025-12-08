package mq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// InMemoryQueue implements MessageQueue using Go channels
// It supports multiple producers (Streamers) and multiple consumers (Collectors)
// and is safe for concurrent use
type InMemoryQueue struct {
	// subscribers maps topics to their handlers
	subscribers map[string][]MessageHandler
	subscribersMu sync.RWMutex

	// messages is the channel for all published messages
	messages chan *Message

	// wg tracks active message processors
	wg sync.WaitGroup

	// ctx and cancel for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// bufferSize is the channel buffer size
	bufferSize int

	// running tracks if the queue is started
	running atomic.Bool

	// closed tracks if the queue is closed
	closed atomic.Bool

	// Statistics (using atomic for thread-safety)
	totalPublished atomic.Int64
	totalDelivered atomic.Int64
	totalErrors    atomic.Int64

	// workerPool limits concurrent message processing
	workerPool chan struct{}
	maxWorkers int
}

// InMemoryQueueConfig holds configuration for the queue
type InMemoryQueueConfig struct {
	BufferSize int // Size of the message channel buffer
	MaxWorkers int // Maximum concurrent message processors
}

// DefaultInMemoryQueueConfig returns default configuration
func DefaultInMemoryQueueConfig() InMemoryQueueConfig {
	return InMemoryQueueConfig{
		BufferSize: 1000,
		MaxWorkers: 100,
	}
}

// NewInMemoryQueue creates a new in-memory message queue
func NewInMemoryQueue(config InMemoryQueueConfig) *InMemoryQueue {
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 100
	}

	return &InMemoryQueue{
		subscribers: make(map[string][]MessageHandler),
		messages:    make(chan *Message, config.BufferSize),
		bufferSize:  config.BufferSize,
		maxWorkers:  config.MaxWorkers,
		workerPool:  make(chan struct{}, config.MaxWorkers),
	}
}

// Start begins processing messages from the queue
func (q *InMemoryQueue) Start(ctx context.Context) error {
	if q.closed.Load() {
		return fmt.Errorf("queue is closed")
	}

	if q.running.Swap(true) {
		return fmt.Errorf("queue is already running")
	}

	q.ctx, q.cancel = context.WithCancel(ctx)

	// Start message dispatcher
	q.wg.Add(1)
	go q.processMessages()

	slog.Info("Message queue started",
		"buffer_size", q.bufferSize,
		"max_workers", q.maxWorkers,
	)

	return nil
}

// Stop gracefully stops the queue
// Waits for all in-flight messages to complete
func (q *InMemoryQueue) Stop() error {
	if !q.running.Load() {
		return nil
	}

	slog.Info("Stopping message queue...")

	// Signal shutdown
	if q.cancel != nil {
		q.cancel()
	}

	// Wait for all message processors to finish
	q.wg.Wait()

	q.running.Store(false)

	slog.Info("Message queue stopped",
		"total_published", q.totalPublished.Load(),
		"total_delivered", q.totalDelivered.Load(),
		"total_errors", q.totalErrors.Load(),
	)

	return nil
}

// Close releases all resources
func (q *InMemoryQueue) Close() error {
	if q.closed.Swap(true) {
		return nil // Already closed
	}

	// Stop processing first
	if err := q.Stop(); err != nil {
		return err
	}

	// Close the messages channel
	close(q.messages)

	slog.Info("Message queue closed")
	return nil
}

// Publish publishes a message to the queue
// Supports multiple concurrent publishers (producers)
func (q *InMemoryQueue) Publish(ctx context.Context, msg *Message) error {
	if q.closed.Load() {
		return fmt.Errorf("queue is closed")
	}

	if !q.running.Load() {
		return fmt.Errorf("queue is not started")
	}

	select {
	case q.messages <- msg:
		q.totalPublished.Add(1)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("publish cancelled: %w", ctx.Err())
	case <-q.ctx.Done():
		return fmt.Errorf("queue is shutting down")
	}
}

// Subscribe registers a handler for a specific topic
// Multiple handlers can subscribe to the same topic
// Thread-safe for concurrent subscriptions
func (q *InMemoryQueue) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	if q.closed.Load() {
		return fmt.Errorf("queue is closed")
	}

	q.subscribersMu.Lock()
	defer q.subscribersMu.Unlock()

	q.subscribers[topic] = append(q.subscribers[topic], handler)

	slog.Info("Handler subscribed to topic",
		"topic", topic,
		"total_handlers", len(q.subscribers[topic]),
	)

	return nil
}

// Unsubscribe removes all handlers for a topic
func (q *InMemoryQueue) Unsubscribe(topic string) error {
	q.subscribersMu.Lock()
	defer q.subscribersMu.Unlock()

	delete(q.subscribers, topic)

	slog.Info("Unsubscribed from topic", "topic", topic)
	return nil
}

// Stats returns current queue statistics
func (q *InMemoryQueue) Stats() QueueStats {
	q.subscribersMu.RLock()
	activeSubscribers := len(q.subscribers)
	q.subscribersMu.RUnlock()

	return QueueStats{
		TotalPublished:    q.totalPublished.Load(),
		TotalDelivered:    q.totalDelivered.Load(),
		TotalErrors:       q.totalErrors.Load(),
		ActiveSubscribers: activeSubscribers,
		QueueDepth:        len(q.messages),
	}
}

// processMessages is the main message dispatcher
// Runs in a dedicated goroutine
func (q *InMemoryQueue) processMessages() {
	defer q.wg.Done()

	for {
		select {
		case msg, ok := <-q.messages:
			if !ok {
				// Channel closed
				return
			}
			q.dispatchMessage(msg)

		case <-q.ctx.Done():
			// Graceful shutdown: drain remaining messages
			q.drainMessages()
			return
		}
	}
}

// dispatchMessage sends a message to all subscribers of its topic
// Uses worker pool to limit concurrent handlers
func (q *InMemoryQueue) dispatchMessage(msg *Message) {
	q.subscribersMu.RLock()
	handlers := q.subscribers[msg.Topic]
	q.subscribersMu.RUnlock()

	if len(handlers) == 0 {
		slog.Warn("No subscribers for topic",
			"topic", msg.Topic,
			"message_id", msg.ID,
		)
		return
	}

	// Dispatch to all handlers concurrently
	for _, handler := range handlers {
		q.wg.Add(1)

		// Acquire worker slot (blocks if pool is full)
		q.workerPool <- struct{}{}

		go q.executeHandler(handler, msg)
	}
}

// executeHandler executes a single handler with error handling
func (q *InMemoryQueue) executeHandler(handler MessageHandler, msg *Message) {
	defer func() {
		q.wg.Done()
		<-q.workerPool // Release worker slot

		// Recover from panics
		if r := recover(); r != nil {
			q.totalErrors.Add(1)
			slog.Error("Handler panicked",
				"panic", r,
				"topic", msg.Topic,
				"message_id", msg.ID,
			)
		}
	}()

	if err := handler(q.ctx, msg); err != nil {
		q.totalErrors.Add(1)
		slog.Error("Handler error",
			"error", err,
			"topic", msg.Topic,
			"message_id", msg.ID,
		)
	} else {
		q.totalDelivered.Add(1)
	}
}

// drainMessages processes remaining messages during shutdown
func (q *InMemoryQueue) drainMessages() {
	for {
		select {
		case msg, ok := <-q.messages:
			if !ok {
				return
			}
			q.dispatchMessage(msg)
		default:
			// No more messages
			return
		}
	}
}
