package queueservice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPServer provides HTTP API for the queue service
type HTTPServer struct {
	queue  *CompetingConsumerQueue
	server *http.Server
	logger *slog.Logger
}

// PublishRequest represents a publish request
type PublishRequest struct {
	Topic    string                 `json:"topic" binding:"required"`
	Payload  json.RawMessage        `json:"payload" binding:"required"`
	Metadata map[string]interface{} `json:"metadata"`
}

// PublishResponse represents a publish response
type PublishResponse struct {
	MessageID string    `json:"message_id"`
	Topic     string    `json:"topic"`
	Timestamp time.Time `json:"timestamp"`
}

// AckRequest represents an acknowledgment request
type AckRequest struct {
	Topic         string `json:"topic" binding:"required"`
	ConsumerGroup string `json:"consumer_group" binding:"required"`
	MessageID     string `json:"message_id" binding:"required"`
}

// NackRequest represents a negative acknowledgment request
type NackRequest struct {
	Topic         string `json:"topic" binding:"required"`
	ConsumerGroup string `json:"consumer_group" binding:"required"`
	MessageID     string `json:"message_id" binding:"required"`
}

// SubscribeRequest query parameters
type SubscribeRequest struct {
	Topic         string `form:"topic" binding:"required"`
	ConsumerGroup string `form:"consumer_group" binding:"required"`
	ConsumerID    string `form:"consumer_id" binding:"required"`
}

// NewHTTPServer creates a new HTTP server for the queue
func NewHTTPServer(queue *CompetingConsumerQueue, port int, logger *slog.Logger) *HTTPServer {
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	hs := &HTTPServer{
		queue:  queue,
		logger: logger.With("component", "http_server"),
	}

	// Register routes
	api := router.Group("/api/v1/queue")
	{
		api.POST("/publish", hs.handlePublish)
		api.GET("/subscribe", hs.handleSubscribe)
		api.POST("/ack", hs.handleAck)
		api.POST("/nack", hs.handleNack)
		api.GET("/stats", hs.handleStats)
	}

	router.GET("/health", hs.handleHealth)

	hs.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return hs
}

// Start starts the HTTP server
func (hs *HTTPServer) Start() error {
	hs.logger.Info("Starting HTTP server", "addr", hs.server.Addr)
	if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server
func (hs *HTTPServer) Shutdown(ctx context.Context) error {
	hs.logger.Info("Shutting down HTTP server")
	return hs.server.Shutdown(ctx)
}

// handlePublish handles message publishing
func (hs *HTTPServer) handlePublish(c *gin.Context) {
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg, err := hs.queue.Publish(c.Request.Context(), req.Topic, req.Payload, req.Metadata)
	if err != nil {
		hs.logger.Error("Failed to publish message",
			"topic", req.Topic,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, PublishResponse{
		MessageID: msg.ID,
		Topic:     msg.Topic,
		Timestamp: msg.Timestamp,
	})
}

// handleSubscribe handles consumer subscriptions via Server-Sent Events (SSE)
func (hs *HTTPServer) handleSubscribe(c *gin.Context) {
	var req SubscribeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create consumer group if it doesn't exist
	if err := hs.queue.CreateConsumerGroup(req.Topic, req.ConsumerGroup); err != nil {
		hs.logger.Error("Failed to create consumer group",
			"topic", req.Topic,
			"group", req.ConsumerGroup,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Register consumer
	consumer, err := hs.queue.RegisterConsumer(req.Topic, req.ConsumerGroup, req.ConsumerID)
	if err != nil {
		hs.logger.Error("Failed to register consumer",
			"topic", req.Topic,
			"group", req.ConsumerGroup,
			"consumer_id", req.ConsumerID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Setup SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Get response writer
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// Send initial connection success event
	fmt.Fprintf(w, "event: connected\ndata: {\"consumer_id\":\"%s\"}\n\n", req.ConsumerID)
	flusher.Flush()

	hs.logger.Info("Consumer connected",
		"topic", req.Topic,
		"group", req.ConsumerGroup,
		"consumer_id", req.ConsumerID,
	)

	// Create context that cancels when client disconnects
	ctx := c.Request.Context()
	clientGone := ctx.Done()

	// Stream messages to consumer
	for {
		select {
		case msg, ok := <-consumer.MessageChan:
			if !ok {
				// Channel closed, consumer unregistered
				hs.logger.Info("Consumer channel closed",
					"consumer_id", req.ConsumerID,
				)
				return
			}

			// Convert message to JSON
			data, err := json.Marshal(msg)
			if err != nil {
				hs.logger.Error("Failed to marshal message",
					"message_id", msg.ID,
					"error", err,
				)
				continue
			}

			// Send SSE event
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()

			hs.logger.Debug("Message sent to consumer",
				"message_id", msg.ID,
				"consumer_id", req.ConsumerID,
			)

		case <-clientGone:
			// Client disconnected
			hs.logger.Info("Consumer disconnected",
				"topic", req.Topic,
				"group", req.ConsumerGroup,
				"consumer_id", req.ConsumerID,
			)

			// Unregister consumer
			if err := hs.queue.UnregisterConsumer(req.Topic, req.ConsumerGroup, req.ConsumerID); err != nil {
				hs.logger.Error("Failed to unregister consumer",
					"consumer_id", req.ConsumerID,
					"error", err,
				)
			}
			return

		case <-time.After(30 * time.Second):
			// Send keepalive ping
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleAck handles message acknowledgment
func (hs *HTTPServer) handleAck(c *gin.Context) {
	var req AckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := hs.queue.Ack(req.Topic, req.ConsumerGroup, req.MessageID); err != nil {
		if err == ErrMessageNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		hs.logger.Error("Failed to acknowledge message",
			"topic", req.Topic,
			"group", req.ConsumerGroup,
			"message_id", req.MessageID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "acknowledged"})
}

// handleNack handles negative acknowledgment
func (hs *HTTPServer) handleNack(c *gin.Context) {
	var req NackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := hs.queue.Nack(req.Topic, req.ConsumerGroup, req.MessageID); err != nil {
		if err == ErrMessageNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		hs.logger.Error("Failed to nack message",
			"topic", req.Topic,
			"group", req.ConsumerGroup,
			"message_id", req.MessageID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "requeued"})
}

// handleStats returns queue statistics
func (hs *HTTPServer) handleStats(c *gin.Context) {
	stats := hs.queue.Stats()
	c.JSON(http.StatusOK, stats)
}

// handleHealth returns health status
func (hs *HTTPServer) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"queue":     hs.queue.String(),
	})
}
