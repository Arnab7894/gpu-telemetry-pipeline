package mq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemoryQueue_BasicPublishSubscribe tests basic pub/sub functionality
func TestInMemoryQueue_BasicPublishSubscribe(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	// Track received messages
	received := make(chan *Message, 10)

	// Subscribe
	err = queue.Subscribe(ctx, "test.topic", func(ctx context.Context, msg *Message) error {
		received <- msg
		return nil
	})
	require.NoError(t, err)

	// Publish a message
	msg, err := NewMessage("test.topic", map[string]string{"key": "value"})
	require.NoError(t, err)

	err = queue.Publish(ctx, msg)
	require.NoError(t, err)

	// Wait for message
	select {
	case receivedMsg := <-received:
		assert.Equal(t, msg.ID, receivedMsg.ID)
		assert.Equal(t, "test.topic", receivedMsg.Topic)

		var payload map[string]string
		err := receivedMsg.Unmarshal(&payload)
		require.NoError(t, err)
		assert.Equal(t, "value", payload["key"])

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// Verify stats
	stats := queue.Stats()
	assert.Equal(t, int64(1), stats.TotalPublished)
	assert.Equal(t, int64(1), stats.TotalDelivered)
	assert.Equal(t, int64(0), stats.TotalErrors)
}

// TestInMemoryQueue_MultipleProducers tests multiple concurrent publishers
func TestInMemoryQueue_MultipleProducers(t *testing.T) {
	queue := NewInMemoryQueue(InMemoryQueueConfig{
		BufferSize: 500,
		MaxWorkers: 50,
	})
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	numProducers := 10
	messagesPerProducer := 100
	expectedTotal := numProducers * messagesPerProducer

	// Track received messages
	var receivedCount atomic.Int64
	received := make(chan string, expectedTotal)

	// Subscribe
	err = queue.Subscribe(ctx, "multi.producer", func(ctx context.Context, msg *Message) error {
		receivedCount.Add(1)
		received <- msg.ID
		return nil
	})
	require.NoError(t, err)

	// Start multiple producers
	var producerWg sync.WaitGroup
	for i := 0; i < numProducers; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()

			for j := 0; j < messagesPerProducer; j++ {
				msg, err := NewMessage("multi.producer", map[string]interface{}{
					"producer": producerID,
					"sequence": j,
				})
				if err != nil {
					t.Errorf("Failed to create message: %v", err)
					return
				}

				if err := queue.Publish(ctx, msg); err != nil {
					t.Errorf("Failed to publish: %v", err)
					return
				}
			}
		}(i)
	}

	// Wait for all producers to finish
	producerWg.Wait()

	// Wait for all messages to be processed
	timeout := time.After(10 * time.Second)
	for receivedCount.Load() < int64(expectedTotal) {
		select {
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages", receivedCount.Load(), expectedTotal)
		case <-time.After(10 * time.Millisecond):
			// Keep waiting
		}
	}

	// Verify all messages received
	assert.Equal(t, int64(expectedTotal), receivedCount.Load())

	stats := queue.Stats()
	assert.Equal(t, int64(expectedTotal), stats.TotalPublished)
	assert.Equal(t, int64(expectedTotal), stats.TotalDelivered)
}

// TestInMemoryQueue_MultipleConsumers tests multiple subscribers on same topic
func TestInMemoryQueue_MultipleConsumers(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	numConsumers := 5
	numMessages := 50

	// Each consumer tracks its own received count
	consumerCounts := make([]atomic.Int64, numConsumers)

	// Subscribe multiple consumers to the same topic
	for i := 0; i < numConsumers; i++ {
		consumerID := i
		err = queue.Subscribe(ctx, "multi.consumer", func(ctx context.Context, msg *Message) error {
			consumerCounts[consumerID].Add(1)
			return nil
		})
		require.NoError(t, err)
	}

	// Publish messages
	for i := 0; i < numMessages; i++ {
		msg, err := NewMessage("multi.consumer", map[string]int{"index": i})
		require.NoError(t, err)

		err = queue.Publish(ctx, msg)
		require.NoError(t, err)
	}

	// Wait for all messages to be processed
	time.Sleep(1 * time.Second)

	// Each consumer should receive all messages (fanout)
	for i := 0; i < numConsumers; i++ {
		assert.Equal(t, int64(numMessages), consumerCounts[i].Load(),
			"Consumer %d received wrong count", i)
	}

	stats := queue.Stats()
	assert.Equal(t, int64(numMessages), stats.TotalPublished)
	// Total delivered = numMessages * numConsumers (fanout)
	assert.Equal(t, int64(numMessages*numConsumers), stats.TotalDelivered)
}

// TestInMemoryQueue_MultipleProducersConsumers tests both together
func TestInMemoryQueue_MultipleProducersConsumers(t *testing.T) {
	queue := NewInMemoryQueue(InMemoryQueueConfig{
		BufferSize: 1000,
		MaxWorkers: 100,
	})
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	numProducers := 10
	numConsumers := 5
	messagesPerProducer := 50
	expectedTotal := numProducers * messagesPerProducer

	// Track total received across all consumers
	var totalReceived atomic.Int64

	// Start multiple consumers
	for i := 0; i < numConsumers; i++ {
		err = queue.Subscribe(ctx, "stress.test", func(ctx context.Context, msg *Message) error {
			totalReceived.Add(1)
			// Simulate some processing time
			time.Sleep(1 * time.Millisecond)
			return nil
		})
		require.NoError(t, err)
	}

	// Start multiple producers
	var producerWg sync.WaitGroup
	for i := 0; i < numProducers; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()

			for j := 0; j < messagesPerProducer; j++ {
				msg, err := NewMessage("stress.test", map[string]int{
					"producer": producerID,
					"message":  j,
				})
				if err != nil {
					return
				}

				if err := queue.Publish(ctx, msg); err != nil {
					return
				}
			}
		}(i)
	}

	producerWg.Wait()

	// Wait for all messages to be processed
	timeout := time.After(15 * time.Second)
	expectedReceived := int64(expectedTotal * numConsumers)
	for totalReceived.Load() < expectedReceived {
		select {
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages",
				totalReceived.Load(), expectedReceived)
		case <-time.After(50 * time.Millisecond):
			// Keep waiting
		}
	}

	assert.Equal(t, expectedReceived, totalReceived.Load())
}

// TestInMemoryQueue_GracefulShutdown tests graceful shutdown behavior
func TestInMemoryQueue_GracefulShutdown(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)

	numMessages := 100
	var processedCount atomic.Int64
	processingStarted := make(chan struct{})
	var startOnce sync.Once

	// Subscribe with slow handler
	err = queue.Subscribe(ctx, "shutdown.test", func(ctx context.Context, msg *Message) error {
		startOnce.Do(func() {
			close(processingStarted)
		})
		processedCount.Add(1)
		// Simulate processing time
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	// Publish messages
	for i := 0; i < numMessages; i++ {
		msg, err := NewMessage("shutdown.test", map[string]int{"index": i})
		require.NoError(t, err)

		err = queue.Publish(ctx, msg)
		require.NoError(t, err)
	}

	// Wait for processing to start
	<-processingStarted

	// Trigger graceful shutdown
	err = queue.Close()
	require.NoError(t, err)

	// Verify all messages were processed before shutdown
	assert.Equal(t, int64(numMessages), processedCount.Load(),
		"Not all messages were processed during graceful shutdown")

	stats := queue.Stats()
	assert.Equal(t, int64(numMessages), stats.TotalPublished)
	assert.Equal(t, int64(numMessages), stats.TotalDelivered)
}

// TestInMemoryQueue_ErrorHandling tests error handling in handlers
func TestInMemoryQueue_ErrorHandling(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	var successCount, errorCount atomic.Int64

	// Subscribe handler that returns errors
	err = queue.Subscribe(ctx, "error.test", func(ctx context.Context, msg *Message) error {
		var payload map[string]int
		if err := msg.Unmarshal(&payload); err != nil {
			return err
		}

		if payload["fail"] == 1 {
			errorCount.Add(1)
			return fmt.Errorf("simulated error")
		}

		successCount.Add(1)
		return nil
	})
	require.NoError(t, err)

	// Publish successful messages
	for i := 0; i < 10; i++ {
		msg, err := NewMessage("error.test", map[string]int{"fail": 0})
		require.NoError(t, err)
		queue.Publish(ctx, msg)
	}

	// Publish failing messages
	for i := 0; i < 5; i++ {
		msg, err := NewMessage("error.test", map[string]int{"fail": 1})
		require.NoError(t, err)
		queue.Publish(ctx, msg)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int64(10), successCount.Load())
	assert.Equal(t, int64(5), errorCount.Load())

	stats := queue.Stats()
	assert.Equal(t, int64(15), stats.TotalPublished)
	assert.Equal(t, int64(10), stats.TotalDelivered) // Only successful ones
	assert.Equal(t, int64(5), stats.TotalErrors)
}

// TestInMemoryQueue_PanicRecovery tests panic recovery in handlers
func TestInMemoryQueue_PanicRecovery(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	var processedCount atomic.Int64

	// Handler that panics
	err = queue.Subscribe(ctx, "panic.test", func(ctx context.Context, msg *Message) error {
		processedCount.Add(1)
		panic("intentional panic")
	})
	require.NoError(t, err)

	// Publish messages
	for i := 0; i < 5; i++ {
		msg, err := NewMessage("panic.test", map[string]int{"index": i})
		require.NoError(t, err)
		queue.Publish(ctx, msg)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// All messages should be processed (panic recovered)
	assert.Equal(t, int64(5), processedCount.Load())

	stats := queue.Stats()
	assert.Equal(t, int64(5), stats.TotalPublished)
	assert.Equal(t, int64(5), stats.TotalErrors) // Panics counted as errors
}

// TestInMemoryQueue_MultiplTopics tests multiple topics
func TestInMemoryQueue_MultipleTopics(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	topic1Count := atomic.Int64{}
	topic2Count := atomic.Int64{}

	// Subscribe to topic1
	err = queue.Subscribe(ctx, "topic1", func(ctx context.Context, msg *Message) error {
		topic1Count.Add(1)
		return nil
	})
	require.NoError(t, err)

	// Subscribe to topic2
	err = queue.Subscribe(ctx, "topic2", func(ctx context.Context, msg *Message) error {
		topic2Count.Add(1)
		return nil
	})
	require.NoError(t, err)

	// Publish to topic1
	for i := 0; i < 10; i++ {
		msg, _ := NewMessage("topic1", map[string]int{"index": i})
		queue.Publish(ctx, msg)
	}

	// Publish to topic2
	for i := 0; i < 15; i++ {
		msg, _ := NewMessage("topic2", map[string]int{"index": i})
		queue.Publish(ctx, msg)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int64(10), topic1Count.Load())
	assert.Equal(t, int64(15), topic2Count.Load())

	stats := queue.Stats()
	assert.Equal(t, int64(25), stats.TotalPublished)
	assert.Equal(t, 2, stats.ActiveSubscribers)
}

// TestInMemoryQueue_ContextCancellation tests context cancellation
func TestInMemoryQueue_ContextCancellation(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx, cancel := context.WithCancel(context.Background())

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	// Subscribe a handler
	var receivedCount atomic.Int64
	err = queue.Subscribe(ctx, "test.cancel", func(ctx context.Context, msg *Message) error {
		receivedCount.Add(1)
		return nil
	})
	require.NoError(t, err)

	// Publish a message before cancellation (should succeed)
	msg, err := NewMessage("test.cancel", map[string]string{"key": "value"})
	require.NoError(t, err)
	err = queue.Publish(ctx, msg)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Give queue time to detect cancellation and shut down
	time.Sleep(200 * time.Millisecond)

	// Try to publish after shutdown (should fail)
	msg2, err := NewMessage("test.cancel", map[string]string{"key": "value2"})
	require.NoError(t, err)

	publishCtx, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	err = queue.Publish(publishCtx, msg2)
	assert.Error(t, err) // Should error because queue is shutting down
}

// TestInMemoryQueue_Unsubscribe tests unsubscribing from topics
func TestInMemoryQueue_Unsubscribe(t *testing.T) {
	queue := NewInMemoryQueue(DefaultInMemoryQueueConfig())
	ctx := context.Background()

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Close()

	var receivedCount atomic.Int64

	// Subscribe
	err = queue.Subscribe(ctx, "unsub.test", func(ctx context.Context, msg *Message) error {
		receivedCount.Add(1)
		return nil
	})
	require.NoError(t, err)

	// Publish and verify receipt
	msg, _ := NewMessage("unsub.test", map[string]string{"key": "value"})
	queue.Publish(ctx, msg)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int64(1), receivedCount.Load())

	// Unsubscribe
	err = queue.Unsubscribe("unsub.test")
	require.NoError(t, err)

	// Publish again (should not be received)
	msg2, _ := NewMessage("unsub.test", map[string]string{"key": "value2"})
	queue.Publish(ctx, msg2)
	time.Sleep(100 * time.Millisecond)

	// Count should not increase
	assert.Equal(t, int64(1), receivedCount.Load())
}
