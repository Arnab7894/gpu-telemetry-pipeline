package mq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPQueueClient implements MessageQueue interface using HTTP API
type HTTPQueueClient struct {
	baseURL       string
	consumerGroup string
	consumerID    string
	httpClient    *http.Client
	logger        *slog.Logger
	subscriptions map[string]*subscription
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

type subscription struct {
	topic    string
	handler  MessageHandler
	conn     *http.Response
	cancel   context.CancelFunc
	doneChan chan struct{}
}

// HTTPQueueConfig configuration for HTTP queue client
type HTTPQueueConfig struct {
	BaseURL       string
	ConsumerGroup string
	ConsumerID    string
	Timeout       time.Duration
}

// NewHTTPQueueClient creates a new HTTP-based queue client
func NewHTTPQueueClient(config HTTPQueueConfig, logger *slog.Logger) *HTTPQueueClient {
	if logger == nil {
		logger = slog.Default()
	}

	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute // Increased to allow full batch delivery
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HTTPQueueClient{
		baseURL:       strings.TrimSuffix(config.BaseURL, "/"),
		consumerGroup: config.ConsumerGroup,
		consumerID:    config.ConsumerID,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:        logger.With("component", "http_queue_client"),
		subscriptions: make(map[string]*subscription),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start initializes the queue client
func (c *HTTPQueueClient) Start(ctx context.Context) error {
	c.logger.Info("HTTP queue client started",
		"base_url", c.baseURL,
		"consumer_group", c.consumerGroup,
		"consumer_id", c.consumerID,
	)
	return nil
}

// Publish publishes a message to a topic
func (c *HTTPQueueClient) Publish(ctx context.Context, msg *Message) error {
	url := fmt.Sprintf("%s/api/v1/queue/publish", c.baseURL)

	// Send the entire message object to preserve ID, timestamp, and headers
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal publish request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Debug("Message published",
		"topic", msg.Topic,
		"message_id", msg.ID,
	)

	return nil
}

// Subscribe subscribes to a topic with a message handler
func (c *HTTPQueueClient) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	c.mu.Lock()
	if _, exists := c.subscriptions[topic]; exists {
		c.mu.Unlock()
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}
	c.mu.Unlock()

	subCtx, subCancel := context.WithCancel(ctx)

	sub := &subscription{
		topic:    topic,
		handler:  handler,
		cancel:   subCancel,
		doneChan: make(chan struct{}),
	}

	c.mu.Lock()
	c.subscriptions[topic] = sub
	c.mu.Unlock()

	// Start batch/prefetch polling in goroutine
	go c.handleBatchPolling(subCtx, sub)

	c.logger.Info("Subscribed to topic",
		"topic", topic,
		"consumer_group", c.consumerGroup,
		"consumer_id", c.consumerID,
	)

	return nil
}

// handleBatchPolling polls for messages using batch/prefetch API (RabbitMQ-style)
func (c *HTTPQueueClient) handleBatchPolling(ctx context.Context, sub *subscription) {
	defer close(sub.doneChan)

	prefetchCount := 500                   // Number of messages to fetch per request
	pollInterval := 100 * time.Millisecond // How often to poll when no messages

	c.logger.Info("Starting batch polling",
		"topic", sub.topic,
		"prefetch_count", prefetchCount,
	)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Subscription cancelled", "topic", sub.topic)
			return
		default:
		}

		// Fetch batch of messages
		url := fmt.Sprintf("%s/api/v1/queue/fetch?topic=%s&consumer_group=%s&consumer_id=%s&prefetch=%d",
			c.baseURL, sub.topic, c.consumerGroup, c.consumerID, prefetchCount)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			c.logger.Error("Failed to create fetch request",
				"topic", sub.topic,
				"error", err,
			)
			time.Sleep(pollInterval)
			continue
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Error("Failed to fetch messages",
				"topic", sub.topic,
				"error", err,
			)
			time.Sleep(pollInterval)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.logger.Error("Fetch failed",
				"topic", sub.topic,
				"status", resp.StatusCode,
				"body", string(bodyBytes),
			)
			time.Sleep(pollInterval)
			continue
		}

		// Parse batch response
		var batchResp struct {
			Messages []*Message `json:"messages"`
			Count    int        `json:"count"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
			resp.Body.Close()
			c.logger.Error("Failed to decode batch response",
				"topic", sub.topic,
				"error", err,
			)
			time.Sleep(pollInterval)
			continue
		}
		resp.Body.Close()

		if batchResp.Count == 0 {
			// No messages available, wait before polling again
			time.Sleep(pollInterval)
			continue
		}

		c.logger.Debug("Batch fetched",
			"topic", sub.topic,
			"count", batchResp.Count,
		)

		// Process all messages in batch
		for _, msg := range batchResp.Messages {
			// Handle message
			if err := sub.handler(ctx, msg); err != nil {
				c.logger.Error("Handler error",
					"topic", sub.topic,
					"message_id", msg.ID,
					"error", err,
				)
				// NACK the message
				if ackErr := c.Nack(sub.topic, msg.ID); ackErr != nil {
					c.logger.Error("Failed to NACK message",
						"message_id", msg.ID,
						"error", ackErr,
					)
				}
			} else {
				// ACK the message
				if ackErr := c.Ack(sub.topic, msg.ID); ackErr != nil {
					c.logger.Error("Failed to ACK message",
						"message_id", msg.ID,
						"error", ackErr,
					)
				}
			}
		}

		// After processing batch, immediately poll for next batch (no delay)
	}
}

// handleSSEConnection handles the Server-Sent Events stream (DEPRECATED - kept for backward compatibility)
func (c *HTTPQueueClient) handleSSEConnection(ctx context.Context, sub *subscription, url string) {
	defer close(sub.doneChan)

	retryDelay := 1 * time.Second
	maxRetryDelay := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Subscription cancelled", "topic", sub.topic)
			return
		default:
		}

		// Create request with context
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			c.logger.Error("Failed to create subscribe request",
				"topic", sub.topic,
				"error", err,
			)
			time.Sleep(retryDelay)
			continue
		}

		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")

		// Make request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Error("Failed to connect to queue service",
				"topic", sub.topic,
				"error", err,
			)
			time.Sleep(retryDelay)
			retryDelay = min(retryDelay*2, maxRetryDelay)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.logger.Error("Subscribe failed",
				"topic", sub.topic,
				"status", resp.StatusCode,
				"body", string(bodyBytes),
			)
			time.Sleep(retryDelay)
			retryDelay = min(retryDelay*2, maxRetryDelay)
			continue
		}

		sub.conn = resp
		retryDelay = 1 * time.Second // Reset retry delay on successful connection

		c.logger.Info("SSE connection established", "topic", sub.topic)

		// Read SSE stream
		scanner := bufio.NewScanner(resp.Body)
		var eventType string
		var eventData []byte

		for scanner.Scan() {
			line := scanner.Text()

			// Check for context cancellation
			select {
			case <-ctx.Done():
				resp.Body.Close()
				return
			default:
			}

			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				eventData = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			} else if line == "" {
				// Empty line indicates end of event
				if eventType == "message" && len(eventData) > 0 {
					c.handleMessage(ctx, sub, eventData)
				}
				eventType = ""
				eventData = nil
			}
		}

		resp.Body.Close()

		if err := scanner.Err(); err != nil {
			c.logger.Error("Error reading SSE stream",
				"topic", sub.topic,
				"error", err,
			)
		}

		// Connection closed, retry
		c.logger.Warn("SSE connection closed, reconnecting", "topic", sub.topic)
		time.Sleep(retryDelay)
	}
}

// handleMessage processes a received message
func (c *HTTPQueueClient) handleMessage(ctx context.Context, sub *subscription, data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error("Failed to unmarshal message",
			"topic", sub.topic,
			"error", err,
		)
		return
	}

	c.logger.Debug("Message received",
		"topic", sub.topic,
		"message_id", msg.ID,
	)

	// Call handler
	if err := sub.handler(ctx, &msg); err != nil {
		c.logger.Error("Handler returned error",
			"topic", sub.topic,
			"message_id", msg.ID,
			"error", err,
		)
		// NACK the message
		if nackErr := c.Nack(sub.topic, msg.ID); nackErr != nil {
			c.logger.Error("Failed to NACK message",
				"message_id", msg.ID,
				"error", nackErr,
			)
		}
		return
	}

	// ACK the message
	if err := c.Ack(sub.topic, msg.ID); err != nil {
		c.logger.Error("Failed to ACK message",
			"message_id", msg.ID,
			"error", err,
		)
	}
}

// Ack acknowledges a message
func (c *HTTPQueueClient) Ack(topic, messageID string) error {
	url := fmt.Sprintf("%s/api/v1/queue/ack", c.baseURL)

	payload := map[string]string{
		"topic":          topic,
		"consumer_group": c.consumerGroup,
		"message_id":     messageID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal ack request: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create ack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ack failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Debug("Message acknowledged", "message_id", messageID)
	return nil
}

// Nack negatively acknowledges a message (requeue)
func (c *HTTPQueueClient) Nack(topic, messageID string) error {
	url := fmt.Sprintf("%s/api/v1/queue/nack", c.baseURL)

	payload := map[string]string{
		"topic":          topic,
		"consumer_group": c.consumerGroup,
		"message_id":     messageID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal nack request: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create nack request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to nack message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nack failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.logger.Debug("Message requeued", "message_id", messageID)
	return nil
}

// Unsubscribe unsubscribes from a topic
func (c *HTTPQueueClient) Unsubscribe(topic string) error {
	c.mu.Lock()
	sub, exists := c.subscriptions[topic]
	if !exists {
		c.mu.Unlock()
		return errors.New("not subscribed to topic")
	}
	delete(c.subscriptions, topic)
	c.mu.Unlock()

	// Cancel subscription context
	sub.cancel()

	// Wait for subscription goroutine to finish
	<-sub.doneChan

	// Close connection if open
	if sub.conn != nil {
		sub.conn.Body.Close()
	}

	c.logger.Info("Unsubscribed from topic", "topic", topic)
	return nil
}

// Stats returns queue statistics (via HTTP API)
func (c *HTTPQueueClient) Stats() QueueStats {
	url := fmt.Sprintf("%s/api/v1/queue/stats", c.baseURL)

	req, err := http.NewRequestWithContext(c.ctx, "GET", url, nil)
	if err != nil {
		c.logger.Error("Failed to create stats request", "error", err)
		return QueueStats{}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Failed to get stats", "error", err)
		return QueueStats{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return QueueStats{}
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		c.logger.Error("Failed to decode stats", "error", err)
		return QueueStats{}
	}

	// Convert to QueueStats (simplified)
	return QueueStats{
		TotalPublished: 0, // Not tracked by HTTP client
		TotalDelivered: 0, // Not tracked by HTTP client
	}
}

// Shutdown closes all subscriptions and shuts down the client
func (c *HTTPQueueClient) Shutdown(ctx context.Context) error {
	c.logger.Info("Shutting down HTTP queue client")

	c.cancel() // Cancel all subscription contexts

	// Wait for all subscriptions to close
	c.mu.RLock()
	subs := make([]*subscription, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subs = append(subs, sub)
	}
	c.mu.RUnlock()

	for _, sub := range subs {
		select {
		case <-sub.doneChan:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	c.logger.Info("HTTP queue client shutdown complete")
	return nil
}

// Stop implements MessageQueue interface (alias for Shutdown)
func (c *HTTPQueueClient) Stop() error {
	return c.Shutdown(context.Background())
}

// Close implements MessageQueue interface (alias for Shutdown)
func (c *HTTPQueueClient) Close() error {
	return c.Shutdown(context.Background())
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
