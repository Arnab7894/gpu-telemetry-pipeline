package queueservice

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/stretchr/testify/assert"
)

func getTestServer(t *testing.T) *HTTPServer {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create RedisQueue - will skip Redis tests if connection fails
	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available for testing")
	}

	server := NewHTTPServer(queue, 8080, logger)
	t.Cleanup(func() {
		queue.Close()
	})

	return server
}

func TestNewHTTPServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		t.Skip("Redis not available for testing")
	}
	defer queue.Close()

	server := NewHTTPServer(queue, 8080, logger)

	assert.NotNil(t, server)
	assert.NotNil(t, server.queue)
	assert.NotNil(t, server.logger)
	assert.NotNil(t, server.server)
	assert.Equal(t, ":8080", server.server.Addr)
}

func TestHTTPServer_Health(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestHTTPServer_PublishInvalidJSON(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/publish", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHTTPServer_PublishValidMessage(t *testing.T) {
	server := getTestServer(t)

	msg, err := mq.NewMessage("test-topic", map[string]string{"key": "value"})
	assert.NoError(t, err)

	body, _ := json.Marshal(msg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/queue/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	// Should succeed since we have a valid message structure
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHTTPServer_PublishMethodNotAllowed(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/publish", nil)
	w := httptest.NewRecorder()

	server.handlePublish(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_SubscribeMethodNotAllowed(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/subscribe", nil)
	w := httptest.NewRecorder()

	server.handleSubscribe(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_FetchMethodNotAllowed(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/fetch", nil)
	w := httptest.NewRecorder()

	server.handleFetch(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_AckMethodNotAllowed(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/ack", nil)
	w := httptest.NewRecorder()

	server.handleAck(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_NackMethodNotAllowed(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/nack", nil)
	w := httptest.NewRecorder()

	server.handleNack(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHTTPServer_StatsMissingTopic(t *testing.T) {
	server := getTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/queue/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHTTPServer_LoggingMiddleware(t *testing.T) {
	server := getTestServer(t)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Wrap with logging middleware
	wrapped := server.loggingMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test", w.Body.String())
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)

	assert.Equal(t, http.StatusCreated, rw.statusCode)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func BenchmarkHTTPServer_HandleHealth(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue, err := NewRedisQueue("redis://localhost:6379", 30*time.Second, 3, logger)
	if err != nil {
		b.Skip("Redis not available for testing")
	}
	defer queue.Close()

	server := NewHTTPServer(queue, 8080, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		server.handleHealth(w, req)
	}
}
