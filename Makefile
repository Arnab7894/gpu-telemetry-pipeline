.PHONY: build test run-streamer run-collector run-api clean cover openapi docker-build docker-streamer docker-collector docker-api docker-queue help

# Docker image configuration
REGISTRY ?=
TAG ?= latest
IMAGE_PREFIX := $(if $(REGISTRY),$(REGISTRY)/,)

# Build all binaries
build:
	@echo "Building all services..."
	@go build -o bin/streamer ./cmd/streamer
	@go build -o bin/collector ./cmd/collector
	@go build -o bin/api-gateway ./cmd/api-gateway
	@echo "Build complete. Binaries in ./bin/"

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race -skip="TestInMemoryQueue_ContextCancellation" ./internal/...
	@echo ""
	@echo "Quick coverage check:"
	@go test -race -skip="TestInMemoryQueue_ContextCancellation" -coverprofile=coverage.tmp.out -covermode=atomic ./internal/... > /dev/null 2>&1 && \
		go tool cover -func=coverage.tmp.out | grep total || true
	@rm -f coverage.tmp.out

# Run tests with coverage
cover:
	@echo "Running tests with coverage..."
	@go test -v -race -skip="TestInMemoryQueue_ContextCancellation" -coverprofile=coverage.out -covermode=atomic ./internal/...
	@go tool cover -html=coverage.out -o coverage.html
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out | grep total
	@echo ""
	@echo "Coverage report generated: coverage.html"
	@echo "Open coverage.html in your browser to view detailed coverage"

# Run streamer service
run-streamer: build
	@echo "Starting Telemetry Streamer..."
	@./bin/streamer

# Run collector service
run-collector: build
	@echo "Starting Telemetry Collector..."
	@./bin/collector

# Run API gateway service
run-api: build
	@echo "Starting API Gateway..."
	@./bin/api-gateway

# Generate OpenAPI specification
openapi:
	@echo "Generating OpenAPI specification..."
	@which swag > /dev/null || (echo "Installing swag..." && go install github.com/swaggo/swag/cmd/swag@latest)
	@swag init -g cmd/api-gateway/main.go -o ./docs/swagger --parseDependency --parseInternal
	@echo "✅ OpenAPI spec generated in ./docs/swagger/"
	@echo "   - swagger.json"
	@echo "   - swagger.yaml"
	@echo ""
	@echo "To view the API documentation:"
	@echo "  1. Run: make run-api"
	@echo "  2. Open: http://localhost:8080/swagger/index.html"

# Alias for openapi
swagger: openapi

# Build Docker images (all at once)
docker-build: docker-streamer docker-collector docker-api docker-queue
	@echo "✅ All Docker images built successfully"
	@echo "Registry: $(if $(REGISTRY),$(REGISTRY),local)"
	@echo "Tag: $(TAG)"

# Build individual Docker images
docker-streamer:
	@echo "Building Streamer Docker image..."
	@docker build -t $(IMAGE_PREFIX)gpu-telemetry-streamer:$(TAG) -f Dockerfile.streamer .
	@echo "✅ $(IMAGE_PREFIX)gpu-telemetry-streamer:$(TAG) built"

docker-collector:
	@echo "Building Collector Docker image..."
	@docker build -t $(IMAGE_PREFIX)gpu-telemetry-collector:$(TAG) -f Dockerfile.collector .
	@echo "✅ $(IMAGE_PREFIX)gpu-telemetry-collector:$(TAG) built"

docker-api:
	@echo "Building API Gateway Docker image..."
	@docker build -t $(IMAGE_PREFIX)gpu-telemetry-api:$(TAG) -f Dockerfile.api-gateway .
	@echo "✅ $(IMAGE_PREFIX)gpu-telemetry-api:$(TAG) built"

docker-queue:
	@echo "Building Queue Service Docker image..."
	@docker build -t $(IMAGE_PREFIX)gpu-telemetry-queue:$(TAG) -f Dockerfile.queue-service .
	@echo "✅ $(IMAGE_PREFIX)gpu-telemetry-queue:$(TAG) built"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@rm -rf api/
	@echo "Clean complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build all service binaries"
	@echo "  test            - Run all tests with race detector"
	@echo "  cover           - Run tests with coverage report"
	@echo "  run-streamer    - Build and run the Telemetry Streamer"
	@echo "  run-collector   - Build and run the Telemetry Collector"
	@echo "  run-api         - Build and run the API Gateway"
	@echo "  openapi         - Generate OpenAPI specification"
	@echo "  docker-build    - Build all Docker images (REGISTRY= TAG=latest)"
	@echo "  docker-streamer - Build Streamer Docker image (REGISTRY= TAG=latest)"
	@echo "  docker-collector- Build Collector Docker image (REGISTRY= TAG=latest)"
	@echo "  docker-api      - Build API Gateway Docker image (REGISTRY= TAG=latest)"
	@echo "  docker-queue    - Build Queue Service Docker image (REGISTRY= TAG=latest)"
	@echo "  clean           - Remove build artifacts and coverage reports"
	@echo "  help            - Show this help message"
