#!/bin/bash

# E2E System Test Runner
# This script sets up the environment and runs the Go system test

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== GPU Telemetry Pipeline E2E Test Runner ===${NC}\n"

# Configuration
API_URL="http://localhost:8080"
PORT_FORWARD_PID=""

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    if [ ! -z "$PORT_FORWARD_PID" ]; then
        kill $PORT_FORWARD_PID 2>/dev/null || true
        echo "Stopped port-forward"
    fi
}

trap cleanup EXIT

# Step 1: Check prerequisites
echo -e "${YELLOW}Step 1: Checking prerequisites...${NC}"
command -v kubectl >/dev/null 2>&1 || { echo -e "${RED}kubectl is required but not installed${NC}"; exit 1; }
command -v go >/dev/null 2>&1 || { echo -e "${RED}go is required but not installed${NC}"; exit 1; }
echo -e "${GREEN}✓ Prerequisites OK${NC}\n"

# Step 2: Verify Kubernetes cluster is accessible
echo -e "${YELLOW}Step 2: Verifying Kubernetes cluster...${NC}"
if ! kubectl cluster-info &>/dev/null; then
    echo -e "${RED}Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Cluster accessible${NC}\n"

# Step 3: Check if pods are running
echo -e "${YELLOW}Step 3: Checking pod status...${NC}"
echo "Pods:"
kubectl get pods -o wide

if ! kubectl get pods | grep -q "Running"; then
    echo -e "${RED}No running pods found. Please deploy the application first.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Pods are running${NC}\n"

# Step 4: Set up port forwarding
echo -e "${YELLOW}Step 4: Setting up port forwarding...${NC}"

# Check if port-forward is already running
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Port 8080 already in use (assuming port-forward is active)${NC}\n"
else
    echo "Starting port-forward to API Gateway..."
    kubectl port-forward service/gpu-telemetry-api-gateway 8080:8080 >/dev/null 2>&1 &
    PORT_FORWARD_PID=$!
    
    # Wait for port-forward to be ready
    echo "Waiting for port-forward to be ready..."
    for i in {1..10}; do
        if curl -s "${API_URL}/health" >/dev/null 2>&1; then
            echo -e "${GREEN}✓ Port-forward ready${NC}\n"
            break
        fi
        if [ $i -eq 10 ]; then
            echo -e "${RED}Port-forward failed to start${NC}"
            exit 1
        fi
        sleep 1
    done
fi

# Step 5: Run the Go system test
echo -e "${YELLOW}Step 5: Running Go system test...${NC}\n"
cd test/e2e
go run system_test.go

# Check exit code
if [ $? -eq 0 ]; then
    echo -e "\n${GREEN}=== All Tests Passed! ===${NC}"
    exit 0
else
    echo -e "\n${RED}=== Some Tests Failed ===${NC}"
    exit 1
fi
