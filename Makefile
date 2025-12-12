.PHONY: build test system-test run-streamer run-collector run-api clean cover openapi docker-build docker-streamer docker-collector docker-api docker-queueservice local-deploy local-cleanup help

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
	@go build -o bin/queueservice ./cmd/queueservice
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

# Run system tests (end-to-end)
system-test:
	@echo "Running system tests..."
	@echo "⚠️  NOTE: Tests expect API Gateway at localhost:8080"
	@echo "⚠️  If testing remote cluster, ensure port-forwarding is active:"
	@echo "    kubectl port-forward service/api-gateway 8080:8080 -n default"
	@echo ""
	@chmod +x system-test.sh
	@./system-test.sh

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
docker-build: docker-streamer docker-collector docker-api docker-queueservice
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

docker-queueservice:
	@echo "Building Queue Service Docker image..."
	@docker build -t $(IMAGE_PREFIX)gpu-telemetry-queueservice:$(TAG) -f Dockerfile.queue-service .
	@echo "✅ $(IMAGE_PREFIX)gpu-telemetry-queueservice:$(TAG) built"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@rm -rf api/
	@echo "Clean complete"

# Local deployment with kind and Helm
local-deploy: CLUSTER_NAME ?= gpu-telemetry-cluster
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
	@kind load docker-image gpu-telemetry-queueservice:latest --name $(CLUSTER_NAME)
	@echo "✅ Images loaded successfully"
	@echo ""
	@echo "[4/7] Installing Helm charts..."
	@echo "  Installing MongoDB..."
	@helm upgrade --install mongodb ./charts/mongodb \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing Redis..."
	@helm upgrade --install redis ./charts/redis \
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
local-cleanup: CLUSTER_NAME ?= gpu-telemetry-cluster
local-cleanup:
	@echo "Cleaning up local deployment..."
	@pkill -f "port-forward.*api-gateway" || true
	@kind delete cluster --name $(CLUSTER_NAME) || true
	@echo "✅ Cleanup complete"

# Deploy to existing Kubernetes cluster
k8s-deploy: NAMESPACE ?= default
k8s-deploy:
	@echo "=========================================="
	@echo "GPU Telemetry Pipeline - K8s Deployment"
	@echo "=========================================="
	@echo ""
	@echo "Checking prerequisites..."
	@echo ""
	@echo "1. Checking Docker..."
	@which docker > /dev/null || (echo "   ❌ Docker CLI not found. Please install Docker." && exit 1)
	@if ! docker info > /dev/null 2>&1; then \
		echo "   ❌ Docker is not running. Please start Docker Desktop or Docker daemon."; \
		exit 1; \
	fi
	@echo "   ✅ Docker is running"
	@echo ""
	@echo "2. Checking kubectl..."
	@which kubectl > /dev/null || (echo "   ❌ kubectl not found. Please install: brew install kubectl" && exit 1)
	@echo "   ✅ kubectl is installed"
	@echo ""
	@echo "3. Checking Helm..."
	@which helm > /dev/null || (echo "   ❌ Helm not found. Please install: brew install helm" && exit 1)
	@echo "   ✅ Helm is installed"
	@echo ""
	@echo "4. Checking Kubernetes cluster connection..."
	@if ! kubectl cluster-info > /dev/null 2>&1; then \
		echo "   ❌ Not connected to a Kubernetes cluster."; \
		echo ""; \
		echo "   Please connect to your Kubernetes cluster using one of:"; \
		echo "     • kubectl config use-context <context-name>"; \
		echo "     • export KUBECONFIG=/path/to/kubeconfig"; \
		echo "     • aws eks update-kubeconfig --name <cluster-name>"; \
		echo "     • gcloud container clusters get-credentials <cluster-name>"; \
		echo ""; \
		echo "   To view available contexts: kubectl config get-contexts"; \
		exit 1; \
	fi
	@echo "   ✅ Connected to Kubernetes cluster:"
	@kubectl cluster-info | head -1 | sed 's/^/      /'
	@echo ""
	@echo "=========================================="
	@echo "5. Checking cluster type..."
	@if kubectl config current-context 2>/dev/null | grep -q "^kind-"; then \
		echo "   ⚠️  Detected kind cluster!"; \
		echo ""; \
		echo "   For kind clusters, use 'make local-deploy' instead."; \
		echo "   It will automatically:"; \
		echo "     • Create/recreate the kind cluster"; \
		echo "     • Load Docker images into the cluster"; \
		echo "     • Deploy all services"; \
		echo ""; \
		echo "   Run: make local-deploy"; \
		echo ""; \
		read -p "   Continue with k8s-deploy anyway? (y/N) " -n 1 -r; \
		echo ""; \
		if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
			exit 1; \
		fi; \
		echo "   ⚠️  WARNING: You'll need to manually load images into kind:"; \
		echo "      kind load docker-image gpu-telemetry-streamer:latest --name <cluster-name>"; \
		echo "      kind load docker-image gpu-telemetry-collector:latest --name <cluster-name>"; \
		echo "      kind load docker-image gpu-telemetry-api:latest --name <cluster-name>"; \
		echo "      kind load docker-image gpu-telemetry-queueservice:latest --name <cluster-name>"; \
	else \
		echo "   ✅ Non-kind cluster detected"; \
	fi
	@echo ""
	@echo "=========================================="
	@echo "All prerequisites validated!"
	@echo "=========================================="
	@echo ""
	@echo "[1/6] Building Docker images..."
	@$(MAKE) docker-build TAG=latest
	@echo ""
	@echo "[2/6] Pushing Docker images to registry..."
	@if [ -z "$(REGISTRY)" ]; then \
		echo "⚠️  WARNING: REGISTRY is not set!"; \
		echo ""; \
		echo "For non-local clusters, you need to push images to a registry."; \
		echo ""; \
		echo "Options:"; \
		echo "  1. For local/kind clusters: Images are already loaded locally"; \
		echo "  2. For remote clusters: Set REGISTRY and push images:"; \
		echo "     make k8s-deploy REGISTRY=<your-registry> TAG=<tag>"; \
		echo ""; \
		echo "Examples:"; \
		echo "  • Docker Hub:  make k8s-deploy REGISTRY=yourusername TAG=v1.0.0"; \
		echo "  • GCR:         make k8s-deploy REGISTRY=gcr.io/project-id TAG=v1.0.0"; \
		echo "  • ECR:         make k8s-deploy REGISTRY=123456789.dkr.ecr.region.amazonaws.com TAG=v1.0.0"; \
		echo ""; \
		read -p "Continue without pushing to registry? (y/N) " -n 1 -r; \
		echo ""; \
		if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
			exit 1; \
		fi; \
	else \
		echo "Pushing images to $(REGISTRY)..."; \
		docker push $(IMAGE_PREFIX)gpu-telemetry-streamer:$(TAG) && \
		docker push $(IMAGE_PREFIX)gpu-telemetry-collector:$(TAG) && \
		docker push $(IMAGE_PREFIX)gpu-telemetry-api:$(TAG) && \
		docker push $(IMAGE_PREFIX)gpu-telemetry-queueservice:$(TAG) && \
		echo "✅ All images pushed successfully"; \
	fi
	@echo ""
	@echo "[3/6] Installing Helm charts..."
	@echo "  Installing MongoDB..."
	@helm upgrade --install mongodb ./charts/mongodb \
		--namespace $(NAMESPACE) \
		--wait --timeout 5m
	@echo "  Installing Redis..."
	@helm upgrade --install redis ./charts/redis \
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
	@echo "[4/6] Waiting for all pods to be ready (this may take a minute)..."
	@kubectl wait --for=condition=ready pod --all \
		--namespace $(NAMESPACE) --timeout=300s || true
	@echo ""
	@kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "[5/6] Setting up port forwarding..."
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
	@echo ""

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build all service binaries"
	@echo "  test            - Run all tests with race detector"
	@echo "  system-test     - Run end-to-end system tests (requires running cluster)"
	@echo "  cover           - Run tests with coverage report"
	@echo "  run-streamer    - Build and run the Telemetry Streamer"
	@echo "  run-collector   - Build and run the Telemetry Collector"
	@echo "  run-api         - Build and run the API Gateway"
	@echo "  openapi         - Generate OpenAPI specification"
	@echo "  docker-build    - Build all Docker images (REGISTRY= TAG=latest)"
	@echo "  docker-streamer - Build Streamer Docker image (REGISTRY= TAG=latest)"
	@echo "  docker-collector- Build Collector Docker image (REGISTRY= TAG=latest)"
	@echo "  docker-api      - Build API Gateway Docker image (REGISTRY= TAG=latest)"
	@echo "  local-deploy    - Deploy entire stack to local kind cluster (CLUSTER_NAME=gpu-telemetry-cluster)"
	@echo "  local-cleanup   - Clean up local kind cluster and port-forwards"
	@echo "  clean           - Remove build artifacts and coverage reports"
	@echo "  help            - Show this help message"
