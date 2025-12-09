.PHONY: build test run-streamer run-collector run-api clean cover openapi docker-build docker-streamer docker-collector docker-api docker-queue local-deploy local-cleanup help

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

# Local deployment with kind and Helm
local-deploy: CLUSTER_NAME ?= gpu-telemetry
local-deploy: NAMESPACE ?= default
local-deploy: CSV_FILE ?= data/dcgm_metrics_20250718_134233.csv
local-deploy:
	@echo "=========================================="
	@echo "GPU Telemetry Pipeline - Local Deployment"
	@echo "=========================================="
	@echo ""
	@echo "Prerequisites:"
	@echo "  ✓ Docker - Container runtime"
	@echo "  ✓ kind - Kubernetes in Docker (https://kind.sigs.k8s.io/)"
	@echo "  ✓ kubectl - Kubernetes CLI (https://kubernetes.io/docs/tasks/tools/)"
	@echo "  ✓ Helm - Kubernetes package manager (https://helm.sh/)"
	@echo ""
	@echo "Checking prerequisites..."
	@which docker > /dev/null || (echo "❌ Docker not found. Please install Docker." && exit 1)
	@which kind > /dev/null || (echo "❌ kind not found. Please install kind: brew install kind" && exit 1)
	@which kubectl > /dev/null || (echo "❌ kubectl not found. Please install kubectl: brew install kubectl" && exit 1)
	@which helm > /dev/null || (echo "❌ Helm not found. Please install Helm: brew install helm" && exit 1)
	@echo "✅ All prerequisites met!"
	@echo ""
	@echo "[1/7] Building Docker images..."
	@$(MAKE) docker-build TAG=latest
	@echo ""
	@echo "[2/7] Creating kind cluster: $(CLUSTER_NAME)..."
	@if kind get clusters 2>/dev/null | grep -q "^$(CLUSTER_NAME)$$"; then \
		echo "⚠️  Cluster '$(CLUSTER_NAME)' already exists. Deleting..."; \
		kind delete cluster --name $(CLUSTER_NAME); \
	fi
	@kind create cluster --name $(CLUSTER_NAME)
	@echo "✅ Cluster created successfully"
	@echo ""
	@echo "[3/7] Loading Docker images into kind cluster..."
	@kind load docker-image gpu-telemetry-streamer:latest --name $(CLUSTER_NAME)
	@kind load docker-image gpu-telemetry-collector:latest --name $(CLUSTER_NAME)
	@kind load docker-image gpu-telemetry-api:latest --name $(CLUSTER_NAME)
	@kind load docker-image gpu-telemetry-queue:latest --name $(CLUSTER_NAME)
	@echo "✅ Images loaded successfully"
	@echo ""
	@echo "[4/6] Installing Helm charts..."
	@echo "  Installing MongoDB..."
	@helm upgrade --install mongodb ./charts/mongodb \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing Queue Service..."
	@helm upgrade --install queue-service ./charts/queue-service \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing Streamer..."
	@helm upgrade --install streamer ./charts/streamer \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing Collector..."
	@helm upgrade --install collector ./charts/collector \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing API Gateway..."
	@helm upgrade --install api-gateway ./charts/api-gateway \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "✅ All Helm charts installed successfully"
	@echo ""
	@echo "[5/6] Waiting for all pods to be ready (this may take a minute)..."
	@kubectl wait --for=condition=ready pod --all \
		--namespace $(NAMESPACE) --timeout=300s || true
	@echo ""
	@kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "[6/6] Setting up port forwarding..."
	@echo "Starting port-forward in background (API Gateway: 8080)..."
	@pkill -f "port-forward.*api-gateway" || true
	@nohup kubectl port-forward service/api-gateway 8080:8080 -n $(NAMESPACE) > /tmp/port-forward.log 2>&1 & 
	@sleep 3
	@echo "✅ Port forwarding active on localhost:8080"
	@echo ""
	@echo "=========================================="
	@echo "🎉 Deployment Complete!"
	@echo "=========================================="
	@echo ""
	@echo "Verification Commands:"
	@echo "  1. Check pods status:"
	@echo "     kubectl get pods -n $(NAMESPACE)"
	@echo ""
	@echo "  2. Test API Gateway health:"
	@echo "     curl http://localhost:8080/health"
	@echo ""
	@echo "  3. List all GPUs:"
	@echo "     curl http://localhost:8080/api/v1/gpus | jq"
	@echo ""
	@echo "  4. Get GPU telemetry (replace <GPU_UUID> with actual UUID):"
	@echo "     curl http://localhost:8080/api/v1/gpus/<GPU_UUID>/telemetry | jq"
	@echo ""
	@echo "  5. View logs:"
	@echo "     kubectl logs -l app=streamer -n $(NAMESPACE) --tail=50 -f"
	@echo "     kubectl logs -l app=collector -n $(NAMESPACE) --tail=50 -f"
	@echo ""
	@echo "To stop port-forwarding: pkill -f 'port-forward.*api-gateway'"
	@echo "To delete cluster: kind delete cluster --name $(CLUSTER_NAME)"
	@echo ""

# Clean up local deployment
local-cleanup: CLUSTER_NAME ?= gpu-telemetry
local-cleanup:
	@echo "Cleaning up local deployment..."
	@pkill -f "port-forward.*api-gateway" || true
	@kind delete cluster --name $(CLUSTER_NAME) || true
	@echo "✅ Cleanup complete"

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
	@echo "  local-deploy    - Deploy entire stack to local kind cluster (CLUSTER_NAME=gpu-telemetry)"
	@echo "  local-cleanup   - Clean up local kind cluster and port-forwards"
	@echo "  clean           - Remove build artifacts and coverage reports"
	@echo "  help            - Show this help message"
