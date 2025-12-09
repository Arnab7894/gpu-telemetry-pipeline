package mq

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Test helper: check if Redis is available for testing
func setupRedisContainer(t *testing.T) (string, func()) {
	t.Helper()

	// For testing, use a local Redis instance or skip
	// In real deployments, testcontainers or docker-compose would be used
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	// Try to connect to verify Redis is available
	ctx := context.Background()
	testQueue, err := NewRedisQueue(RedisQueueConfig{
		RedisURL:      redisURL,
		ConsumerGroup: "test",
		ConsumerID:    "test",
	}, slog.Default())

	if err != nil {
		t.Skipf("Redis not available for testing (set TEST_REDIS_URL or run Redis on localhost:6379): %v", err)
		return "", func() {}
	}

	testQueue.Close()

	// Cleanup function to flush test data
	cleanup := func() {
		if queue, err := NewRedisQueue(RedisQueueConfig{
			RedisURL:      redisURL,
			ConsumerGroup: "test",
			ConsumerID:    "test",
		}, slog.Default()); err == nil {
			queue.client.FlushDB(ctx)
			queue.Close()
		}
	}

	return redisURL, cleanup
}

// Test helper: create Redis queue for testing
func createTestRedisQueue(t *testing.T, redisURL, consumerGroup, consumerID string) *RedisQueue {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	queue, err := NewRedisQueue(RedisQueueConfig{
		RedisURL:          redisURL,
		ConsumerGroup:     consumerGroup,
		ConsumerID:        consumerID,
		VisibilityTimeout: 2 * time.Second,
		MaxRetries:        3,
	}, logger)

	if err != nil {
		t.Fatalf("Failed to create Redis queue: %v", err)
	}

	ctx := context.Background()
	if err := queue.Start(ctx); err != nil {
		t.Fatalf("Failed to start queue: %v", err)
	}

	return queue
}

func TestRedisQueue_PublishAndSubscribe(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	queue := createTestRedisQueue(t, redisURL, "test-group", "consumer-1")
	defer queue.Close()

	ctx := context.Background()

	// Subscribe to topic
	received := make(chan *Message, 1)
	handler := func(ctx context.Context, msg *Message) error {
		received <- msg
		return nil
	}

	err := queue.Subscribe(ctx, "test-topic", handler)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish message
	testPayload := map[string]string{"test": "data"}
	msg, err := NewMessage("test-topic", testPayload)
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	err = queue.Publish(ctx, msg)
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case receivedMsg := <-received:
		if receivedMsg.ID != msg.ID {
			t.Errorf("Expected message ID %s, got %s", msg.ID, receivedMsg.ID)
		}

		// Verify payload
		var payload map[string]string
		if err := json.Unmarshal(receivedMsg.Payload, &payload); err != nil {
			t.Fatalf("Failed to unmarshal payload: %v", err)
		}

		if payload["test"] != "data" {
			t.Errorf("Expected payload 'data', got '%s'", payload["test"])
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Verify stats
	stats := queue.Stats()
	if stats.TotalPublished != 1 {
		t.Errorf("Expected 1 published message, got %d", stats.TotalPublished)
	}
	if stats.TotalDelivered != 1 {
		t.Errorf("Expected 1 delivered message, got %d", stats.TotalDelivered)
	}
}

func TestRedisQueue_MultipleConsumers(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	// Create two consumers in the same group
	consumer1 := createTestRedisQueue(t, redisURL, "test-group", "consumer-1")
	defer consumer1.Close()

	consumer2 := createTestRedisQueue(t, redisURL, "test-group", "consumer-2")
	defer consumer2.Close()

	ctx := context.Background()

	// Track which consumer received which message
	received1 := make(chan string, 10)
	received2 := make(chan string, 10)

	handler1 := func(ctx context.Context, msg *Message) error {
		received1 <- msg.ID
		return nil
	}

	handler2 := func(ctx context.Context, msg *Message) error {
		received2 <- msg.ID
		return nil
	}

	// Subscribe both consumers
	if err := consumer1.Subscribe(ctx, "test-topic", handler1); err != nil {
		t.Fatalf("Failed to subscribe consumer1: %v", err)
	}

	if err := consumer2.Subscribe(ctx, "test-topic", handler2); err != nil {
		t.Fatalf("Failed to subscribe consumer2: %v", err)
	}

	// Publish multiple messages
	messageCount := 10
	messageIDs := make([]string, messageCount)

	for i := 0; i < messageCount; i++ {
		msg, err := NewMessage("test-topic", map[string]int{"index": i})
		if err != nil {
			t.Fatalf("Failed to create message: %v", err)
		}
		messageIDs[i] = msg.ID

		if err := consumer1.Publish(ctx, msg); err != nil {
			t.Fatalf("Failed to publish: %v", err)
		}
	}

	// Collect received messages
	receivedCount1 := 0
	receivedCount2 := 0
	timeout := time.After(10 * time.Second)

	for receivedCount1+receivedCount2 < messageCount {
		select {
		case <-received1:
			receivedCount1++
		case <-received2:
			receivedCount2++
		case <-timeout:
			t.Fatalf("Timeout: received %d + %d = %d messages, expected %d",
				receivedCount1, receivedCount2, receivedCount1+receivedCount2, messageCount)
		}
	}

	// Verify load distribution (both consumers should get messages)
	t.Logf("Consumer1 received %d messages, Consumer2 received %d messages",
		receivedCount1, receivedCount2)

	if receivedCount1 == 0 || receivedCount2 == 0 {
		t.Error("Load balancing failed - one consumer received no messages")
	}

	if receivedCount1+receivedCount2 != messageCount {
		t.Errorf("Expected %d total messages, got %d", messageCount, receivedCount1+receivedCount2)
	}
}

func TestRedisQueue_VisibilityTimeout(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	// Create queue with short visibility timeout
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	queue, err := NewRedisQueue(RedisQueueConfig{
		RedisURL:          redisURL,
		ConsumerGroup:     "test-group",
		ConsumerID:        "consumer-1",
		VisibilityTimeout: 2 * time.Second, // Short timeout for testing
		MaxRetries:        3,
	}, logger)

	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}
	defer queue.Close()

	ctx := context.Background()
	if err := queue.Start(ctx); err != nil {
		t.Fatalf("Failed to start queue: %v", err)
	}

	// Subscribe with a handler that never completes (simulating stuck processing)
	var processCount atomic.Int32
	handler := func(ctx context.Context, msg *Message) error {
		processCount.Add(1)
		// Simulate long processing by blocking
		<-ctx.Done()
		return ctx.Err()
	}

	if err := queue.Subscribe(ctx, "test-topic", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish a message
	msg, err := NewMessage("test-topic", map[string]string{"test": "timeout"})
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	if err := queue.Publish(ctx, msg); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for visibility timeout + checker interval
	time.Sleep(15 * time.Second)

	// Message should have been redelivered
	count := processCount.Load()
	if count < 2 {
		t.Errorf("Expected at least 2 delivery attempts due to timeout, got %d", count)
	}

	t.Logf("Message was redelivered %d times due to visibility timeout", count)
}

func TestRedisQueue_Nack(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	queue := createTestRedisQueue(t, redisURL, "test-group", "consumer-1")
	defer queue.Close()

	ctx := context.Background()

	// Track delivery count
	var deliveryCount atomic.Int32
	received := make(chan *Message, 5)

	handler := func(ctx context.Context, msg *Message) error {
		count := deliveryCount.Add(1)
		received <- msg

		// Fail first 2 attempts
		if count <= 2 {
			return errors.New("simulated processing error") // Trigger nack/retry
		}
		return nil
	}

	if err := queue.Subscribe(ctx, "test-topic", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish message
	msg, err := NewMessage("test-topic", map[string]string{"test": "nack"})
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	if err := queue.Publish(ctx, msg); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for multiple delivery attempts
	time.Sleep(5 * time.Second)

	count := deliveryCount.Load()
	if count < 3 {
		t.Errorf("Expected at least 3 delivery attempts (initial + retries), got %d", count)
	}

	t.Logf("Message was delivered %d times before success", count)
}

func TestRedisQueue_MaxRetries(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	queue, err := NewRedisQueue(RedisQueueConfig{
		RedisURL:          redisURL,
		ConsumerGroup:     "test-group",
		ConsumerID:        "consumer-1",
		VisibilityTimeout: 1 * time.Second,
		MaxRetries:        2, // Low retry count for testing
	}, logger)

	if err != nil {
		t.Fatalf("Failed to create queue: %v", err)
	}
	defer queue.Close()

	ctx := context.Background()
	if err := queue.Start(ctx); err != nil {
		t.Fatalf("Failed to start queue: %v", err)
	}

	// Handler that always fails
	var attemptCount atomic.Int32
	handler := func(ctx context.Context, msg *Message) error {
		attemptCount.Add(1)
		return errors.New("permanent failure") // Always fail
	}

	if err := queue.Subscribe(ctx, "test-topic", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish message
	msg, err := NewMessage("test-topic", map[string]string{"test": "max-retries"})
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	if err := queue.Publish(ctx, msg); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait enough time for retries
	time.Sleep(10 * time.Second)

	attempts := attemptCount.Load()
	t.Logf("Total delivery attempts: %d", attempts)

	// Should not exceed max retries + initial attempt
	if attempts > int32(queue.maxRetries+1) {
		t.Errorf("Expected at most %d attempts, got %d", queue.maxRetries+1, attempts)
	}
}

func TestRedisQueue_Unsubscribe(t *testing.T) {
	redisURL, cleanup := setupRedisContainer(t)
	defer cleanup()

	queue := createTestRedisQueue(t, redisURL, "test-group", "consumer-1")
	defer queue.Close()

	ctx := context.Background()

	// Subscribe
	received := make(chan *Message, 10)
	handler := func(ctx context.Context, msg *Message) error {
		received <- msg
		return nil
	}

	if err := queue.Subscribe(ctx, "test-topic", handler); err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish a message
	msg1, _ := NewMessage("test-topic", map[string]string{"msg": "1"})
	if err := queue.Publish(ctx, msg1); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case <-received:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for first message")
	}

	// Unsubscribe
	if err := queue.Unsubscribe("test-topic"); err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// Publish another message
	msg2, _ := NewMessage("test-topic", map[string]string{"msg": "2"})
	if err := queue.Publish(ctx, msg2); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Should NOT receive the second message
	select {
	case msg := <-received:
		t.Errorf("Received message after unsubscribe: %s", msg.ID)
	case <-time.After(2 * time.Second):
		// Expected - no message received
	}
}

func TestRedisQueue_ConnectionFailure(t *testing.T) {
	// Try to create queue with invalid Redis URL
	logger := slog.Default()

	_, err := NewRedisQueue(RedisQueueConfig{
		RedisURL:      "redis://invalid-host:9999",
		ConsumerGroup: "test-group",
		ConsumerID:    "consumer-1",
	}, logger)

	if err == nil {
		t.Fatal("Expected error when connecting to invalid Redis URL")
	}

	if !os.IsTimeout(err) && err != ErrRedisConnection {
		t.Logf("Got expected error: %v", err)
	}
}
