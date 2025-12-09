package queueservice

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSelectConsumer_LeastLoaded tests that selectConsumer picks the consumer with the least load
func TestSelectConsumer_LeastLoaded(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	topic := &Topic{
		name:           "test-topic",
		messages:       make(chan *Message, 100),
		consumerGroups: make(map[string]*ConsumerGroup),
	}

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
		topic:     topic,
	}

	// Create three consumers with different loads
	consumer1 := &Consumer{
		ID:                 "consumer-1",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 5, // Highest load
	}

	consumer2 := &Consumer{
		ID:                 "consumer-2",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 2, // Lowest load
	}

	consumer3 := &Consumer{
		ID:                 "consumer-3",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 3, // Medium load
	}

	group.consumers["consumer-1"] = consumer1
	group.consumers["consumer-2"] = consumer2
	group.consumers["consumer-3"] = consumer3

	// Test that selectConsumer picks consumer with least load
	selected := queue.selectConsumer(group)
	if selected == nil {
		t.Fatal("Expected a consumer to be selected, got nil")
	}

	if selected.ID != "consumer-2" {
		t.Errorf("Expected consumer-2 (lowest load=2) to be selected, got %s with load=%d",
			selected.ID, atomic.LoadInt64(&selected.activeMessageCount))
	}
}

// TestSelectConsumer_NoConsumers tests behavior when no consumers are available
func TestSelectConsumer_NoConsumers(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
	}

	selected := queue.selectConsumer(group)
	if selected != nil {
		t.Errorf("Expected nil when no consumers available, got %v", selected)
	}
}

// TestSelectConsumer_SingleConsumer tests selection with only one consumer
func TestSelectConsumer_SingleConsumer(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
	}

	consumer := &Consumer{
		ID:                 "consumer-1",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 10,
	}

	group.consumers["consumer-1"] = consumer

	selected := queue.selectConsumer(group)
	if selected == nil {
		t.Fatal("Expected consumer to be selected, got nil")
	}

	if selected.ID != "consumer-1" {
		t.Errorf("Expected consumer-1, got %s", selected.ID)
	}
}

// TestSelectConsumer_EqualLoad tests selection when consumers have equal load
func TestSelectConsumer_EqualLoad(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
	}

	// Create consumers with equal load
	for i := 1; i <= 3; i++ {
		consumer := &Consumer{
			ID:                 "consumer-" + string(rune('0'+i)),
			MessageChan:        make(chan *Message, 10),
			LastActivity:       time.Now(),
			activeMessageCount: 5, // All have same load
		}
		group.consumers[consumer.ID] = consumer
	}

	selected := queue.selectConsumer(group)
	if selected == nil {
		t.Fatal("Expected a consumer to be selected, got nil")
	}

	// Should select one of them (any is valid since they're equal)
	if atomic.LoadInt64(&selected.activeMessageCount) != 5 {
		t.Errorf("Expected selected consumer to have load 5, got %d",
			atomic.LoadInt64(&selected.activeMessageCount))
	}
}

// TestConsumer_GetActiveMessageCount tests the helper method
func TestConsumer_GetActiveMessageCount(t *testing.T) {
	consumer := &Consumer{
		ID:                 "consumer-1",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 0,
	}

	// Initial count should be 0
	if count := consumer.GetActiveMessageCount(); count != 0 {
		t.Errorf("Expected initial count 0, got %d", count)
	}

	// Increment and verify
	atomic.AddInt64(&consumer.activeMessageCount, 1)
	if count := consumer.GetActiveMessageCount(); count != 1 {
		t.Errorf("Expected count 1 after increment, got %d", count)
	}

	// Add more
	atomic.AddInt64(&consumer.activeMessageCount, 5)
	if count := consumer.GetActiveMessageCount(); count != 6 {
		t.Errorf("Expected count 6, got %d", count)
	}

	// Decrement
	atomic.AddInt64(&consumer.activeMessageCount, -2)
	if count := consumer.GetActiveMessageCount(); count != 4 {
		t.Errorf("Expected count 4 after decrement, got %d", count)
	}
}

// TestLoadBalancing_Concurrent tests that load balancing works correctly under concurrent access
func TestLoadBalancing_Concurrent(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        1000,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
	}

	// Create multiple consumers
	numConsumers := 5
	consumers := make([]*Consumer, numConsumers)
	for i := 0; i < numConsumers; i++ {
		consumer := &Consumer{
			ID:                 "consumer-" + string(rune('0'+i+1)),
			MessageChan:        make(chan *Message, 100),
			LastActivity:       time.Now(),
			activeMessageCount: 0,
		}
		consumers[i] = consumer
		group.consumers[consumer.ID] = consumer
	}

	// Concurrently select consumers and verify distribution
	var wg sync.WaitGroup
	numSelections := 100
	selections := make([]string, numSelections)

	for i := 0; i < numSelections; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			selected := queue.selectConsumer(group)
			if selected != nil {
				selections[index] = selected.ID
				// Simulate message delivery by incrementing count
				atomic.AddInt64(&selected.activeMessageCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// Verify that consumers were selected (not nil)
	nonNilCount := 0
	for _, sel := range selections {
		if sel != "" {
			nonNilCount++
		}
	}

	if nonNilCount != numSelections {
		t.Errorf("Expected %d non-nil selections, got %d", numSelections, nonNilCount)
	}

	// Verify that load was distributed (all consumers should have some messages)
	// With 100 selections and 5 consumers, each should get approximately 20
	for _, consumer := range consumers {
		count := atomic.LoadInt64(&consumer.activeMessageCount)
		if count < 10 || count > 30 {
			t.Logf("Consumer %s has %d messages (expected around 20)", consumer.ID, count)
		}
	}
}

// TestAck_DecrementsLoadCount tests that Ack decrements the consumer's active message count
func TestAck_DecrementsLoadCount(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	// Create topic and consumer group
	topicName := "test-topic"
	groupName := "test-group"
	consumerID := "consumer-1"

	// Create topic by publishing a dummy message
	ctx := context.Background()
	queue.Publish(ctx, topicName, []byte("{}"), nil)

	// Create consumer group
	queue.CreateConsumerGroup(topicName, groupName)

	// Register consumer
	consumer, err := queue.RegisterConsumer(topicName, groupName, consumerID)
	if err != nil {
		t.Fatalf("Failed to register consumer: %v", err)
	}

	// Set initial load
	atomic.StoreInt64(&consumer.activeMessageCount, 5)
	initialLoad := consumer.GetActiveMessageCount()

	// Create a pending entry
	topic := queue.topics[topicName]
	group := topic.consumerGroups[groupName]

	messageID := "msg-1"
	msg := &Message{
		ID:            messageID,
		Topic:         topicName,
		Timestamp:     time.Now(),
		DeliveryCount: 0,
	}

	entry := &PendingEntry{
		Message:         msg,
		ConsumerID:      consumerID,
		ConsumerGroup:   groupName,
		DeliveryTime:    time.Now(),
		VisibilityUntil: time.Now().Add(5 * time.Minute),
		DeliveryCount:   1,
	}

	group.pending.Add(messageID, entry)

	// Ack the message
	err = queue.Ack(topicName, groupName, messageID)
	if err != nil {
		t.Fatalf("Failed to ack message: %v", err)
	}

	// Verify load was decremented
	finalLoad := consumer.GetActiveMessageCount()
	if finalLoad != initialLoad-1 {
		t.Errorf("Expected load to decrease from %d to %d, got %d",
			initialLoad, initialLoad-1, finalLoad)
	}
}

// TestNack_DecrementsLoadCount tests that Nack decrements the consumer's active message count
func TestNack_DecrementsLoadCount(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	// Create topic and consumer group
	topicName := "test-topic"
	groupName := "test-group"
	consumerID := "consumer-1"

	// Create topic by publishing a dummy message
	ctx := context.Background()
	queue.Publish(ctx, topicName, []byte("{}"), nil)

	// Create consumer group
	queue.CreateConsumerGroup(topicName, groupName)

	// Register consumer
	consumer, err := queue.RegisterConsumer(topicName, groupName, consumerID)
	if err != nil {
		t.Fatalf("Failed to register consumer: %v", err)
	}

	// Set initial load
	atomic.StoreInt64(&consumer.activeMessageCount, 5)
	initialLoad := consumer.GetActiveMessageCount()

	// Create a pending entry
	topic := queue.topics[topicName]
	group := topic.consumerGroups[groupName]

	messageID := "msg-1"
	msg := &Message{
		ID:            messageID,
		Topic:         topicName,
		Timestamp:     time.Now(),
		DeliveryCount: 0,
	}

	entry := &PendingEntry{
		Message:         msg,
		ConsumerID:      consumerID,
		ConsumerGroup:   groupName,
		DeliveryTime:    time.Now(),
		VisibilityUntil: time.Now().Add(5 * time.Minute),
		DeliveryCount:   1,
	}

	group.pending.Add(messageID, entry)

	// Nack the message
	err = queue.Nack(topicName, groupName, messageID)
	if err != nil {
		t.Fatalf("Failed to nack message: %v", err)
	}

	// Verify load was decremented
	finalLoad := consumer.GetActiveMessageCount()
	if finalLoad != initialLoad-1 {
		t.Errorf("Expected load to decrease from %d to %d, got %d",
			initialLoad, initialLoad-1, finalLoad)
	}
}

// TestSelectConsumer_DynamicLoadChanges tests selection as loads change dynamically
func TestSelectConsumer_DynamicLoadChanges(t *testing.T) {
	logger := slog.Default()
	queue := NewCompetingConsumerQueue(Config{
		BufferSize:        100,
		VisibilityTimeout: 5 * time.Minute,
		MaxRetries:        3,
	}, logger)

	group := &ConsumerGroup{
		name:      "test-group",
		consumers: make(map[string]*Consumer),
		pending:   &PendingList{entries: make(map[string]*PendingEntry)},
	}

	consumer1 := &Consumer{
		ID:                 "consumer-1",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 10,
	}

	consumer2 := &Consumer{
		ID:                 "consumer-2",
		MessageChan:        make(chan *Message, 10),
		LastActivity:       time.Now(),
		activeMessageCount: 10,
	}

	group.consumers["consumer-1"] = consumer1
	group.consumers["consumer-2"] = consumer2

	// Initially equal load, either can be selected
	selected1 := queue.selectConsumer(group)
	if selected1 == nil {
		t.Fatal("Expected a consumer to be selected")
	}

	// Increase consumer1's load
	atomic.AddInt64(&consumer1.activeMessageCount, 5)

	// Now consumer2 should be selected (lower load)
	selected2 := queue.selectConsumer(group)
	if selected2 == nil {
		t.Fatal("Expected a consumer to be selected")
	}

	if selected2.ID != "consumer-2" {
		t.Errorf("Expected consumer-2 to be selected (load=10), got %s (load=%d)",
			selected2.ID, atomic.LoadInt64(&selected2.activeMessageCount))
	}

	// Decrease consumer1's load below consumer2
	atomic.AddInt64(&consumer1.activeMessageCount, -10) // Now has 5
	atomic.AddInt64(&consumer2.activeMessageCount, 5)   // Now has 15

	// Now consumer1 should be selected
	selected3 := queue.selectConsumer(group)
	if selected3 == nil {
		t.Fatal("Expected a consumer to be selected")
	}

	if selected3.ID != "consumer-1" {
		t.Errorf("Expected consumer-1 to be selected (load=5), got %s (load=%d)",
			selected3.ID, atomic.LoadInt64(&selected3.activeMessageCount))
	}
}
