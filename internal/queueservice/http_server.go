package queueservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
)

// HTTPServer wraps the RedisQueue with HTTP API endpoints
type HTTPServer struct {
	queue  *RedisQueue
	logger *slog.Logger
	server *http.Server
}

// NewHTTPServer creates a new HTTP server for the queue
func NewHTTPServer(queue *RedisQueue, port int, logger *slog.Logger) *HTTPServer {
	s := &HTTPServer{
		queue:  queue,
		logger: logger.With("component", "http_server"),
	}

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/v1/queue/publish", s.handlePublish)
	mux.HandleFunc("/api/v1/queue/subscribe", s.handleSubscribe) // TODO: Remove
	mux.HandleFunc("/api/v1/queue/fetch", s.handleFetch)         // Batch/prefetch endpoint
	mux.HandleFunc("/api/v1/queue/ack", s.handleAck)
	mux.HandleFunc("/api/v1/queue/nack", s.handleNack)
	mux.HandleFunc("/api/v1/queue/stats", s.handleStats)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           s.loggingMiddleware(mux),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	s.logger.Info("Starting HTTP server", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")
	return s.server.Shutdown(ctx)
}

// loggingMiddleware logs HTTP requests
func (s *HTTPServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		s.logger.Debug("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"status", wrapped.statusCode,
			"duration", time.Since(start),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// handlePublish handles POST /api/v1/queue/publish
func (s *HTTPServer) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var msg mq.Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.queue.Publish(r.Context(), &msg); err != nil {
		s.logger.Error("Failed to publish message", "error", err)
		http.Error(w, "Failed to publish message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "success",
		"message_id": msg.ID,
	})
}

// TODO: Remove
// handleSubscribe handles GET /api/v1/queue/subscribe (Server-Sent Events)
func (s *HTTPServer) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get query parameters
	topic := r.URL.Query().Get("topic")
	consumerGroup := r.URL.Query().Get("consumer_group")
	consumerID := r.URL.Query().Get("consumer_id")

	if topic == "" || consumerGroup == "" || consumerID == "" {
		http.Error(w, "Missing required parameters: topic, consumer_group, consumer_id", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Flush headers
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Register consumer
	eventsCh := s.queue.RegisterConsumer(topic, consumerGroup, consumerID)
	defer s.queue.UnregisterConsumer(topic, consumerGroup, consumerID)

	// Send connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"consumer_id\":\"%s\"}\n\n", consumerID)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	s.logger.Info("SSE connection established",
		"topic", topic,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
	)

	// Send messages as they arrive
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("SSE connection closed by client",
				"consumer_id", consumerID,
			)
			return

		case msg, ok := <-eventsCh:
			if !ok {
				// Channel closed
				return
			}

			msgJSON, err := json.Marshal(msg)
			if err != nil {
				s.logger.Error("Failed to marshal message", "error", err)
				continue
			}

			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msgJSON)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprint(w, ": ping\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

// handleFetch handles GET /api/v1/queue/fetch (Prefetch/Batch delivery)
// Returns N messages at once for batch processing
func (s *HTTPServer) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get query parameters
	topic := r.URL.Query().Get("topic")
	consumerGroup := r.URL.Query().Get("consumer_group")
	consumerID := r.URL.Query().Get("consumer_id")
	prefetchStr := r.URL.Query().Get("prefetch")

	if topic == "" || consumerGroup == "" || consumerID == "" {
		http.Error(w, "Missing required parameters: topic, consumer_group, consumer_id", http.StatusBadRequest)
		return
	}

	// Parse prefetch count (default: 500)
	prefetch := 500
	if prefetchStr != "" {
		if parsed, err := fmt.Sscanf(prefetchStr, "%d", &prefetch); err != nil || parsed != 1 {
			http.Error(w, "Invalid prefetch parameter", http.StatusBadRequest)
			return
		}
	}

	// Limit prefetch to reasonable range
	if prefetch < 1 {
		prefetch = 1
	}
	if prefetch > 2500 {
		prefetch = 2500
	}

	s.logger.Debug("Fetch request",
		"topic", topic,
		"consumer_group", consumerGroup,
		"consumer_id", consumerID,
		"prefetch", prefetch,
	)

	// Fetch messages (blocking with timeout)
	ctx := r.Context()
	messages, err := s.queue.FetchBatch(ctx, topic, consumerGroup, consumerID, prefetch)
	if err != nil {
		s.logger.Error("Failed to fetch messages",
			"error", err,
			"topic", topic,
		)
		http.Error(w, fmt.Sprintf("Failed to fetch messages: %v", err), http.StatusInternalServerError)
		return
	}

	// Return messages as JSON array
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": messages,
		"count":    len(messages),
	})
}

// handleAck handles POST /api/v1/queue/ack
func (s *HTTPServer) handleAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Topic         string `json:"topic"`
		ConsumerGroup string `json:"consumer_group"`
		MessageID     string `json:"message_id"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Topic == "" || req.ConsumerGroup == "" || req.MessageID == "" {
		http.Error(w, "Missing required fields: topic, consumer_group, message_id", http.StatusBadRequest)
		return
	}

	if err := s.queue.Ack(req.Topic, req.ConsumerGroup, req.MessageID); err != nil {
		s.logger.Error("Failed to ack message",
			"error", err,
			"message_id", req.MessageID,
		)
		http.Error(w, fmt.Sprintf("Failed to ack message: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "success",
		"message_id": req.MessageID,
	})
}

// handleNack handles POST /api/v1/queue/nack
func (s *HTTPServer) handleNack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Topic         string `json:"topic"`
		ConsumerGroup string `json:"consumer_group"`
		MessageID     string `json:"message_id"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Topic == "" || req.ConsumerGroup == "" || req.MessageID == "" {
		http.Error(w, "Missing required fields: topic, consumer_group, message_id", http.StatusBadRequest)
		return
	}

	if err := s.queue.Nack(req.Topic, req.ConsumerGroup, req.MessageID); err != nil {
		s.logger.Error("Failed to nack message",
			"error", err,
			"message_id", req.MessageID,
		)
		http.Error(w, fmt.Sprintf("Failed to nack message: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "success",
		"message_id": req.MessageID,
	})
}

// handleStats handles GET /api/v1/queue/stats
func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.queue.GetStats()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}

// handleHealth handles GET /health
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "queue-service",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}
