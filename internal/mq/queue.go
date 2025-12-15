package mq

import "context"

// MessageHandler is a function that handles a message
// It receives the message and returns an error if processing fails
type MessageHandler func(ctx context.Context, msg *Message) error

// MessageQueue defines the interface for message queue operations
// This abstraction allows swapping implementations (in-memory, Redis, Kafka, etc.)
// following the Dependency Inversion Principle (SOLID)
type MessageQueue interface {
	// Publish publishes a message to the queue
	// Returns an error if the queue is full or closed
	Publish(ctx context.Context, msg *Message) error

	// Subscribe registers a handler for messages on a specific topic
	// Multiple handlers can subscribe to the same topic
	// Returns an error if subscription fails
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error

	// Unsubscribe removes all handlers for a topic
	Unsubscribe(topic string) error

	// Start begins processing messages
	// Must be called before publishing/subscribing
	Start(ctx context.Context) error

	// Stop gracefully stops processing
	// Waits for in-flight messages to complete
	Stop() error

	// Close releases all resources
	// Should be called after Stop()
	Close() error

	// TODO: Cleanup - Make Stats() optional or remove from interface
	// HTTPQueueClient returns hardcoded zeros because it's a proxy without local tracking.
	// Consider: 1) Making this method optional, 2) Removing it entirely, or
	// 3) Properly mapping Queue Service stats to QueueStats semantics
	// Stats returns queue statistics (optional, for monitoring)
	Stats() QueueStats
}

// QueueStats represents statistics about the queue
type QueueStats struct {
	// TotalPublished is the total number of messages published
	TotalPublished int64

	// TotalDelivered is the total number of messages delivered to handlers
	TotalDelivered int64

	// TotalErrors is the total number of handler errors
	TotalErrors int64

	// ActiveSubscribers is the current number of active subscribers
	ActiveSubscribers int

	// QueueDepth is the current number of messages waiting to be processed
	QueueDepth int
}
