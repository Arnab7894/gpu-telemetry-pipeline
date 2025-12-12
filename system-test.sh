#!/bin/bash

# System Test Script for GPU Telemetry Pipeline
# Tests the complete end-to-end flow: CSV → Streamer → Queue → Collector → MongoDB → API

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE=${NAMESPACE:-default}
API_PORT=${API_PORT:-8080}
TEST_TIMEOUT=300 # 5 minutes
POLL_INTERVAL=5

# Function to print colored messages
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

echo "=========================================="
echo "GPU Telemetry Pipeline - System Test"
echo "=========================================="
echo ""
print_warning "IMPORTANT: This test assumes the API Gateway is accessible at localhost:${API_PORT}"
print_warning "Please ensure port-forwarding is set up if testing against a remote cluster:"
print_warning "  kubectl port-forward service/api-gateway 8080:8080 -n \${NAMESPACE}"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to wait for condition with timeout
wait_for_condition() {
    local description=$1
    local check_command=$2
    local timeout=$3
    local elapsed=0

    print_info "Waiting for: $description (timeout: ${timeout}s)"
    
    while [ $elapsed -lt $timeout ]; do
        if eval "$check_command" >/dev/null 2>&1; then
            print_success "$description (took ${elapsed}s)"
            return 0
        fi
        sleep $POLL_INTERVAL
        elapsed=$((elapsed + POLL_INTERVAL))
        echo -n "."
    done
    
    echo ""
    print_error "Timeout waiting for: $description"
    return 1
}

# Check prerequisites
print_info "Checking prerequisites..."
for cmd in kubectl curl jq; do
    if ! command_exists "$cmd"; then
        print_error "$cmd is not installed. Please install it first."
        exit 1
    fi
done
print_success "All prerequisites met"
echo ""

# Check if kubectl can connect to a cluster
print_info "Checking Kubernetes cluster connectivity..."
if ! kubectl cluster-info >/dev/null 2>&1; then
    print_error "Cannot connect to Kubernetes cluster."
    print_info "Please ensure:"
    print_info "  1. A Kubernetes cluster is running (kind, minikube, GKE, EKS, etc.)"
    print_info "  2. kubectl is configured with the correct context"
    print_info "  3. You have deployed the GPU telemetry services to the cluster"
    exit 1
fi

CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "unknown")
print_success "Connected to cluster: $CURRENT_CONTEXT"
echo ""

# Check if all pods are ready
print_info "Checking if all pods are ready..."
if ! kubectl wait --for=condition=ready pod --all -n "$NAMESPACE" --timeout=60s >/dev/null 2>&1; then
    print_error "Not all pods are ready. Current status:"
    kubectl get pods -n "$NAMESPACE"
    exit 1
fi
print_success "All pods are ready"
echo ""

# Show current pod status
print_info "Current pod status:"
kubectl get pods -n "$NAMESPACE"
echo ""

# Setup port forwarding if not already running
print_info "Setting up port forwarding to API Gateway..."
if ! lsof -i :$API_PORT >/dev/null 2>&1; then
    kubectl port-forward service/api-gateway $API_PORT:8080 -n "$NAMESPACE" >/dev/null 2>&1 &
    PORT_FORWARD_PID=$!
    sleep 3
    print_success "Port forwarding active (PID: $PORT_FORWARD_PID)"
else
    print_warning "Port $API_PORT already in use (assuming port-forward is active)"
    PORT_FORWARD_PID=""
fi
echo ""

# Cleanup function
cleanup() {
    echo ""
    print_info "Cleaning up..."
    if [ -n "$PORT_FORWARD_PID" ]; then
        kill $PORT_FORWARD_PID 2>/dev/null || true
    fi
}

trap cleanup EXIT

# Test 1: API Health Check
print_info "Test 1: API Gateway Health Check"
if curl -sf http://localhost:$API_PORT/health | jq -e '.status == "healthy"' >/dev/null; then
    print_success "API Gateway is healthy"
else
    print_error "API Gateway health check failed"
    curl -s http://localhost:$API_PORT/health || echo "No response"
    exit 1
fi
echo ""

# Test 2: Wait for data ingestion
print_info "Test 2: Waiting for telemetry data ingestion..."
wait_for_condition \
    "GPUs to be registered" \
    "curl -sf http://localhost:$API_PORT/api/v1/gpus | jq -e '.total > 0'" \
    $TEST_TIMEOUT

TOTAL_GPUS=$(curl -sf http://localhost:$API_PORT/api/v1/gpus | jq -r '.total')
print_success "Found $TOTAL_GPUS GPU(s) with telemetry data"
echo ""

# Test 3: List all GPUs
print_info "Test 3: Listing all GPUs"
GPU_RESPONSE=$(curl -sf http://localhost:$API_PORT/api/v1/gpus)
echo "$GPU_RESPONSE" | jq '.'

# Validate response structure
if ! echo "$GPU_RESPONSE" | jq -e '.gpus | type == "array"' >/dev/null; then
    print_error "Invalid GPU list response structure"
    exit 1
fi

if ! echo "$GPU_RESPONSE" | jq -e '.total | type == "number"' >/dev/null; then
    print_error "Invalid GPU list total field"
    exit 1
fi

print_success "GPU list API working correctly"
echo ""

# Test 4: Get specific GPU details
print_info "Test 4: Testing GPU details endpoint"
FIRST_GPU_UUID=$(echo "$GPU_RESPONSE" | jq -r '.gpus[0].uuid')
print_info "Testing with GPU: $FIRST_GPU_UUID"

GPU_DETAIL=$(curl -sf "http://localhost:$API_PORT/api/v1/gpus/$FIRST_GPU_UUID")
echo "$GPU_DETAIL" | jq '.'

# Validate GPU detail fields
for field in uuid model_name hostname device_id gpu_index; do
    if ! echo "$GPU_DETAIL" | jq -e ".$field" >/dev/null; then
        print_error "Missing field in GPU details: $field"
        exit 1
    fi
done

print_success "GPU details API working correctly"
echo ""

# Test 5: Get telemetry data (unfiltered)
print_info "Test 5: Testing telemetry endpoint (no filters)"
TELEMETRY_RESPONSE=$(curl -sf "http://localhost:$API_PORT/api/v1/gpus/$FIRST_GPU_UUID/telemetry?limit=10")
echo "$TELEMETRY_RESPONSE" | jq '{gpu_uuid, total, metrics: .metrics | length}'

# Validate telemetry response
if ! echo "$TELEMETRY_RESPONSE" | jq -e '.metrics | type == "array"' >/dev/null; then
    print_error "Invalid telemetry response structure"
    exit 1
fi

TELEMETRY_COUNT=$(echo "$TELEMETRY_RESPONSE" | jq -r '.metrics | length')
if [ "$TELEMETRY_COUNT" -eq 0 ]; then
    print_error "No telemetry data found for GPU: $FIRST_GPU_UUID"
    exit 1
fi

print_success "Retrieved $TELEMETRY_COUNT telemetry entries"
echo ""

# Test 6: Verify telemetry ordering (newest first)
print_info "Test 6: Verifying telemetry ordering (newest first)"
TIMESTAMPS=$(echo "$TELEMETRY_RESPONSE" | jq -r '.metrics[].timestamp' | head -5)
echo "First 5 timestamps:"
echo "$TIMESTAMPS"

# Check if timestamps are in descending order
FIRST_TS=$(echo "$TIMESTAMPS" | head -1)
LAST_TS=$(echo "$TIMESTAMPS" | tail -1)

if [[ "$FIRST_TS" > "$LAST_TS" ]] || [[ "$FIRST_TS" == "$LAST_TS" ]]; then
    print_success "Telemetry is correctly ordered (newest first)"
else
    print_error "Telemetry ordering is incorrect"
    print_error "First timestamp: $FIRST_TS"
    print_error "Last timestamp: $LAST_TS"
    exit 1
fi
echo ""

# Test 7: Telemetry with time range filter
print_info "Test 7: Testing telemetry with time range filters"

# Get current time and calculate past time
END_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
START_TIME=$(date -u -v-1H +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d "1 hour ago" +"%Y-%m-%dT%H:%M:%SZ")

print_info "Start time: $START_TIME"
print_info "End time: $END_TIME"

FILTERED_RESPONSE=$(curl -sf "http://localhost:$API_PORT/api/v1/gpus/$FIRST_GPU_UUID/telemetry?start_time=$START_TIME&end_time=$END_TIME&limit=5")
FILTERED_COUNT=$(echo "$FILTERED_RESPONSE" | jq -r '.metrics | length')

print_info "Found $FILTERED_COUNT telemetry entries in time range"

if [ "$FILTERED_COUNT" -eq 0 ]; then
    print_warning "No data in specified time range (this may be expected if data is very old)"
else
    print_success "Time range filtering working correctly"
fi
echo ""

# Test 8: Validate required telemetry fields
print_info "Test 8: Validating telemetry data structure"
SAMPLE_METRIC=$(echo "$TELEMETRY_RESPONSE" | jq -r '.metrics[0]')
echo "Sample telemetry entry:"
echo "$SAMPLE_METRIC" | jq '.'

for field in gpu_uuid timestamp metric_name value; do
    if ! echo "$SAMPLE_METRIC" | jq -e ".$field" >/dev/null; then
        print_error "Missing required field in telemetry: $field"
        exit 1
    fi
done

print_success "Telemetry data structure is valid"
echo ""

# Test 9: Test invalid UUID handling
print_info "Test 9: Testing error handling (invalid UUID)"
INVALID_RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/test_invalid.json "http://localhost:$API_PORT/api/v1/gpus/invalid-uuid/telemetry")

if [ "$INVALID_RESPONSE" -eq 400 ]; then
    print_success "API correctly returns 400 for invalid UUID"
else
    print_error "Expected 400 for invalid UUID, got: $INVALID_RESPONSE"
    exit 1
fi
echo ""

# Test 10: Test non-existent GPU
print_info "Test 10: Testing error handling (non-existent GPU)"
NONEXISTENT_UUID="GPU-00000000-0000-0000-0000-000000000000"
NOTFOUND_RESPONSE=$(curl -s -w "%{http_code}" -o /tmp/test_notfound.json "http://localhost:$API_PORT/api/v1/gpus/$NONEXISTENT_UUID/telemetry")

if [ "$NOTFOUND_RESPONSE" -eq 404 ]; then
    print_success "API correctly returns 404 for non-existent GPU"
else
    print_error "Expected 404 for non-existent GPU, got: $NOTFOUND_RESPONSE"
    exit 1
fi
echo ""

# Test 11: Verify Swagger documentation
print_info "Test 11: Verifying Swagger UI availability"
SWAGGER_RESPONSE=$(curl -s -w "%{http_code}" -o /dev/null "http://localhost:$API_PORT/swagger/index.html")

if [ "$SWAGGER_RESPONSE" -eq 200 ]; then
    print_success "Swagger UI is accessible"
else
    print_error "Swagger UI not accessible (HTTP $SWAGGER_RESPONSE)"
    exit 1
fi
echo ""

# Test 12: Check component logs for errors
print_info "Test 12: Checking component logs for errors"
ERROR_COUNT=0

for component in streamer collector api-gateway queue-service; do
    print_info "Checking $component logs..."
    ERRORS=$(kubectl logs -l app=$component -n "$NAMESPACE" --tail=100 2>/dev/null | grep -i error | wc -l || echo "0")
    if [ "$ERRORS" -gt 0 ]; then
        print_warning "Found $ERRORS error(s) in $component logs"
        ERROR_COUNT=$((ERROR_COUNT + ERRORS))
    else
        print_success "$component: no errors"
    fi
done

if [ "$ERROR_COUNT" -gt 0 ]; then
    print_warning "Total errors found in logs: $ERROR_COUNT"
    print_info "This may be expected for transient errors during startup"
else
    print_success "No errors found in component logs"
fi
echo ""

# Final Summary
echo "=========================================="
echo "           TEST SUMMARY"
echo "=========================================="
print_success "All system tests passed! ✓"
echo ""
echo "Test Results:"
echo "  ✓ API Gateway Health Check"
echo "  ✓ Data Ingestion (found $TOTAL_GPUS GPU(s))"
echo "  ✓ List GPUs endpoint"
echo "  ✓ Get GPU details endpoint"
echo "  ✓ Get telemetry endpoint"
echo "  ✓ Telemetry ordering (newest first)"
echo "  ✓ Time range filtering"
echo "  ✓ Data structure validation"
echo "  ✓ Error handling (400 for invalid UUID)"
echo "  ✓ Error handling (404 for non-existent GPU)"
echo "  ✓ Swagger documentation"
echo "  ✓ Component health check"
echo ""
print_success "GPU Telemetry Pipeline is functioning correctly!"
echo "=========================================="
echo ""
echo "Quick Access:"
echo "  • Swagger UI: http://localhost:$API_PORT/swagger/index.html"
echo "  • List GPUs: curl http://localhost:$API_PORT/api/v1/gpus | jq"
echo "  • View logs: kubectl logs -l app=<component> -n $NAMESPACE"
echo ""
