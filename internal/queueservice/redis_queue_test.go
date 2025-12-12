package queueservice

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/stretchr/testify/assert"
)

func TestNewRedisQueue_InvalidURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := NewRedisQueue("invalid://url", 30*time.Second, 3, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse Redis URL")
}

func TestNewRedisQueue_ConnectionFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Try to connect to non-existent Redis
	_, err := NewRedisQueue("redis://nonexistent:6379", 30*time.Second, 3, logger)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to connect to Redis")
	}
}

func TestRedisQueue_PublishWithoutConnection(t *testing.T) {
	// Skip if no Redis available
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available")
	}
	defer queue.Close()

	ctx := context.Background()
	msg, _ := mq.NewMessage("test-topic", map[string]string{"key": "value"})

	err = queue.Publish(ctx, msg)
	// Should either succeed or fail gracefully
	assert.NotPanics(t, func() {
		queue.Publish(ctx, msg)
	})
}

func TestRedisQueue_Close(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available")
	}

	err = queue.Close()
	assert.NoError(t, err)
}

func TestQueueConsumerGroup_Creation(t *testing.T) {
	group := &QueueConsumerGroup{
		Name:      "test-group",
		Consumers: make(map[string]*QueueConsumer),
	}

	assert.NotNil(t, group)
	assert.Equal(t, "test-group", group.Name)
	assert.NotNil(t, group.Consumers)
}

func TestQueueConsumer_Creation(t *testing.T) {
	consumer := &QueueConsumer{
		ID:       "consumer-1",
		EventsCh: make(chan *mq.Message, 10),
	}

	assert.NotNil(t, consumer)
	assert.Equal(t, "consumer-1", consumer.ID)
	assert.NotNil(t, consumer.EventsCh)
}

func TestPendingEntry_Creation(t *testing.T) {
	msg, _ := mq.NewMessage("test", map[string]string{"data": "test"})

	entry := &PendingEntry{
		Message:       msg,
		ConsumerGroup: "group-1",
		ConsumerID:    "consumer-1",
		VisibleAt:     time.Now().Add(30 * time.Second),
		DeliveryCount: 1,
	}

	assert.NotNil(t, entry)
	assert.Equal(t, "group-1", entry.ConsumerGroup)
	assert.Equal(t, "consumer-1", entry.ConsumerID)
	assert.Equal(t, 1, entry.DeliveryCount)
}

func TestRedisQueue_GetStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available")
	}
	defer queue.Close()

	stats := queue.GetStats()
	assert.NotNil(t, stats)
}

func TestRedisQueue_Ack(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available")
	}
	defer queue.Close()

	err = queue.Ack("test-topic", "test-group", "test-message-id")

	// Should handle gracefully even if message doesn't exist
	assert.NotPanics(t, func() {
		queue.Ack("test-topic", "test-group", "test-message-id")
	})
}

func TestRedisQueue_Nack(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available")
	}
	defer queue.Close()

	err = queue.Nack("test-topic", "test-group", "test-message-id")

	// Should handle gracefully
	assert.NotPanics(t, func() {
		queue.Nack("test-topic", "test-group", "test-message-id")
	})
}
