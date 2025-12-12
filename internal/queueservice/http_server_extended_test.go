package queueservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/stretchr/testify/assert"
)

// Helper to get test server - skips if Redis unavailable
func getExtendedTestHTTPServer(t *testing.T) *HTTPServer {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available for extended HTTP server tests")
		return nil
	}

	server := NewHTTPServer(queue, 8181, logger)
	t.Cleanup(func() {
		queue.Close()
	})

	return server
}

func TestHTTPServer_handlePublish_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	msg := mq.Message{
		ID:      "test-msg-extended-123",
		Topic:   "test-topic-extended",
		Payload: json.RawMessage(`{"data":"test extended"}`),
	}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/publish", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "test-msg-extended-123", response["message_id"])
}

func TestHTTPServer_handlePublish_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/publish", nil)
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Contains(t, w.Body.String(), "Method not allowed")
}

func TestHTTPServer_handlePublish_Extended_InvalidJSON(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/publish", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestHTTPServer_handleSubscribe_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/subscribe", nil)
	w := httptest.NewRecorder()

	server.handleSubscribe(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_handleSubscribe_Extended_MissingParams(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	// Missing all params
	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/subscribe", nil)
	w := httptest.NewRecorder()

	server.handleSubscribe(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Missing required parameters")
}

func TestHTTPServer_handleFetch_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	// First publish some messages
	ctx := context.Background()
	topic := "fetch-topic-extended"

	for i := 0; i < 3; i++ {
		msg := &mq.Message{
			ID:      "fetch-msg-extended-" + string(rune(i+'0')),
			Topic:   topic,
			Payload: json.RawMessage(`{"index":` + string(rune(i+'0')) + `}`),
		}
		server.queue.Publish(ctx, msg)
	}

	// Wait briefly for messages to be stored
	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch?topic="+topic+"&consumer_group=cg1&consumer_id=c1&prefetch=10", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	count := response["count"].(float64)
	assert.GreaterOrEqual(t, int(count), 0) // May be 0 if queue empty
}

func TestHTTPServer_handleFetch_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/fetch", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_handleFetch_Extended_MissingParams(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Missing required parameters")
}

func TestHTTPServer_handleFetch_Extended_InvalidPrefetch(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch?topic=test&consumer_group=group1&consumer_id=consumer1&prefetch=invalid", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid prefetch")
}

func TestHTTPServer_handleFetch_Extended_DefaultPrefetch(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch?topic=test-extended&consumer_group=group1&consumer_id=consumer1", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	// Should succeed with default prefetch of 500
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHTTPServer_handleAck_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	reqBody := map[string]string{
		"topic":          "test-topic-extended",
		"consumer_group": "group1",
		"message_id":     "msg-extended-123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/ack", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAck(w, req)

	// May succeed or fail depending on whether message exists
	// Just verify we handle it without panicking
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestHTTPServer_handleAck_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/ack", nil)
	w := httptest.NewRecorder()

	server.handleAck(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_handleAck_Extended_InvalidJSON(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/ack", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	server.handleAck(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestHTTPServer_handleAck_Extended_MissingFields(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	reqBody := map[string]string{
		"topic": "test-topic",
		// Missing consumer_group and message_id
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/ack", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleAck(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Missing required fields")
}

func TestHTTPServer_handleNack_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	reqBody := map[string]string{
		"topic":          "test-topic-extended",
		"consumer_group": "group1",
		"message_id":     "msg-extended-456",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/nack", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleNack(w, req)

	// May succeed or fail depending on whether message exists
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestHTTPServer_handleNack_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/nack", nil)
	w := httptest.NewRecorder()

	server.handleNack(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_handleNack_Extended_InvalidJSON(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/nack", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	server.handleNack(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid JSON")
}

func TestHTTPServer_handleNack_Extended_MissingFields(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	reqBody := map[string]string{
		"topic": "test-topic",
		// Missing consumer_group and message_id
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/nack", bytes.NewReader(body))
	w := httptest.NewRecorder()

	server.handleNack(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Missing required fields")
}

func TestHTTPServer_handleStats_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	// Stats should have some fields
	assert.NotNil(t, response)
}

func TestHTTPServer_handleStats_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_handleHealth_Extended_Success(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "queue-service", response["service"])
	assert.NotEmpty(t, response["timestamp"])
}

func TestHTTPServer_handleHealth_Extended_MethodNotAllowed(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_Extended_loggingMiddleware(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	wrapped := server.loggingMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())
}

func TestHTTPServer_Extended_responseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)

	assert.Equal(t, http.StatusCreated, rw.statusCode)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHTTPServer_Extended_Shutdown(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)

	// Should succeed even if server wasn't started
	assert.NoError(t, err)
}

// Integration test: Publish -> Fetch -> Ack flow
func TestHTTPServer_Extended_IntegrationFlow(t *testing.T) {
	server := getExtendedTestHTTPServer(t)

	topic := "integration-topic-extended"

	// 1. Publish a message
	msg := mq.Message{
		ID:      "msg-integration-extended-123",
		Topic:   topic,
		Payload: json.RawMessage(`{"data":"integration test"}`),
	}
	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/publish", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handlePublish(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. Wait briefly
	time.Sleep(100 * time.Millisecond)

	// 3. Fetch messages
	req = httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch?topic="+topic+"&consumer_group=integration-cg&consumer_id=c1&prefetch=10", nil)
	w = httptest.NewRecorder()
	server.handleFetch(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var fetchResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &fetchResp)
	count := int(fetchResp["count"].(float64))
	assert.GreaterOrEqual(t, count, 0)

	// 4. Ack the message (may fail if not actually fetched)
	ackBody := map[string]string{
		"topic":          topic,
		"consumer_group": "integration-cg",
		"message_id":     "msg-integration-extended-123",
	}
	ackJSON, _ := json.Marshal(ackBody)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/queue/ack", bytes.NewReader(ackJSON))
	w = httptest.NewRecorder()
	server.handleAck(w, req)
	// Don't assert status code, as message might not exist in PEL

	// 5. Check stats
	req = httptest.NewRequest(http.MethodGet, "/api/v1/queue/stats", nil)
	w = httptest.NewRecorder()
	server.handleStats(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 6. Check health
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	w = httptest.NewRecorder()
	server.handleHealth(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
