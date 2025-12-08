# End-to-End Testing Guide

This guide provides step-by-step instructions for testing the GPU Telemetry Pipeline on a local Kubernetes cluster using **kind**.

## Prerequisites

- Docker installed and running
- kind installed
- kubectl installed
- Helm v3.x installed
- Go 1.21+ installed (for optional system test)

## Manual End-to-End Test

### Step 1: Create a kind Cluster

```bash
# Create a new kind cluster
kind create cluster --name gpu-telemetry

# Verify the cluster is ready
kubectl cluster-info --context kind-gpu-telemetry
kubectl get nodes
```

Expected output:
```
NAME                          STATUS   ROLES           AGE   VERSION
gpu-telemetry-control-plane   Ready    control-plane   30s   v1.27.x
```

### Step 2: Build Docker Images and Load into kind

```bash
# Build all Docker images
make docker-build

# Verify images were created
docker images | grep gpu-telemetry
```

Expected output:
```
gpu-telemetry-streamer    latest   <image-id>   ...
gpu-telemetry-collector   latest   <image-id>   ...
gpu-telemetry-api         latest   <image-id>   ...
gpu-telemetry-queue       latest   <image-id>   ...
```

```bash
# Load images into the kind cluster
kind load docker-image gpu-telemetry-streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-api:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-queue:latest --name gpu-telemetry

# Verify images are loaded (run inside kind container)
docker exec -it gpu-telemetry-control-plane crictl images | grep gpu-telemetry
```

### Step 3: Create CSV ConfigMap

The streamer needs the CSV data to stream telemetry. Create a ConfigMap from the CSV file:

```bash
# Create ConfigMap from CSV file
kubectl create configmap gpu-telemetry-csv-data \
  --from-file=metrics.csv=data/dcgm_metrics_20250718_134233.csv

# Verify ConfigMap was created
kubectl get configmap gpu-telemetry-csv-data
kubectl describe configmap gpu-telemetry-csv-data
```

### Step 4: Install the Helm Chart

```bash
# Install the Helm chart with default values (2 streamers, 2 collectors)
helm install gpu-telemetry ./charts/gpu-telemetry

# Or install with custom replica counts
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.replicaCount=3 \
  --set collector.replicaCount=3
```

Expected output:
```
NAME: gpu-telemetry
LAST DEPLOYED: ...
NAMESPACE: default
STATUS: deployed
REVISION: 1
...
```

### Step 5: Verify All Pods are Running

```bash
# Watch pods until all are running (press Ctrl+C to exit)
kubectl get pods -w

# Check final pod status
kubectl get pods
```

Expected output (with 2 streamers and 2 collectors):
```
NAME                                              READY   STATUS    RESTARTS   AGE
gpu-telemetry-api-gateway-xxxxxxxxxx-xxxxx       1/1     Running   0          60s
gpu-telemetry-collector-xxxxxxxxxx-xxxxx         1/1     Running   0          60s
gpu-telemetry-collector-xxxxxxxxxx-yyyyy         1/1     Running   0          60s
gpu-telemetry-queue-service-xxxxxxxxxx-xxxxx     1/1     Running   0          60s
gpu-telemetry-streamer-xxxxxxxxxx-xxxxx          1/1     Running   0          60s
gpu-telemetry-streamer-xxxxxxxxxx-yyyyy          1/1     Running   0          60s
```

```bash
# Verify services are created
kubectl get svc
```

Expected output:
```
NAME                              TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
gpu-telemetry-api-gateway         ClusterIP   10.96.xxx.xxx   <none>        8080/TCP   60s
gpu-telemetry-queue-service       ClusterIP   10.96.xxx.xxx   <none>        8081/TCP   60s
```

### Step 6: Check Component Logs

Verify each component is working correctly:

```bash
# Check streamer logs (should show CSV rows being published)
kubectl logs -l app.kubernetes.io/component=streamer --tail=20

# Check queue service logs (should show messages being enqueued)
kubectl logs -l app.kubernetes.io/component=queue-service --tail=20

# Check collector logs (should show messages being processed and stored)
kubectl logs -l app.kubernetes.io/component=collector --tail=20

# Check API Gateway logs (should show startup and ready state)
kubectl logs -l app.kubernetes.io/component=api-gateway --tail=20
```

Expected streamer log patterns:
```
INFO: Starting Telemetry Streamer instance=gpu-telemetry-streamer-xxx
INFO: Reading CSV file path=/app/data/metrics.csv
INFO: Published telemetry gpu_id=<id> timestamp=<time>
```

Expected collector log patterns:
```
INFO: Starting Telemetry Collector instance=gpu-telemetry-collector-xxx
INFO: Connected to queue url=http://queue-service:8081
INFO: Stored telemetry gpu_id=<id> records=1
```

### Step 7: Port-Forward and Access the API

In a separate terminal, set up port forwarding:

```bash
# Forward local port 8080 to API Gateway service
kubectl port-forward service/gpu-telemetry-api-gateway 8080:8080
```

Keep this terminal running. In another terminal, test the API endpoints:

```bash
# Test 1: Health check
curl http://localhost:8080/health

# Expected: {"status":"ok"}

# Test 2: Get all GPUs
curl http://localhost:8080/api/v1/gpus | jq

# Expected: List of GPUs with their composite keys
# [
#   {
#     "id": "hostname1:GPU-device1",
#     "hostname": "hostname1",
#     "device_id": "GPU-device1",
#     ...
#   },
#   ...
# ]

# Test 3: Get a specific GPU ID from the list
# Copy one of the GPU IDs from the previous response
export GPU_ID="hostname1:GPU-device1"

# Test 4: Get telemetry for a specific GPU
curl "http://localhost:8080/api/v1/gpus/${GPU_ID}/telemetry" | jq

# Expected: Array of telemetry points
# [
#   {
#     "gpu_id": "hostname1:GPU-device1",
#     "timestamp": "2025-12-08T10:30:00Z",
#     "gpu_utilization": 85.5,
#     "memory_utilization": 70.2,
#     ...
#   },
#   ...
# ]

# Test 5: Get telemetry with time range filter
# Set time range (adjust based on when you started the system)
START_TIME=$(date -u -v-5M +"%Y-%m-%dT%H:%M:%SZ")  # 5 minutes ago
END_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")           # now

curl "http://localhost:8080/api/v1/gpus/${GPU_ID}/telemetry?start_time=${START_TIME}&end_time=${END_TIME}" | jq

# Expected: Filtered telemetry points within the time range

# Test 6: View Swagger documentation
open http://localhost:8080/swagger/index.html
# Or visit in your browser
```

### Step 8: Verify Data Flow

Check that the entire pipeline is working:

```bash
# 1. Verify streamers are publishing data
kubectl logs -l app.kubernetes.io/component=streamer --tail=50 | grep "Published"

# 2. Verify collectors are receiving and storing data
kubectl logs -l app.kubernetes.io/component=collector --tail=50 | grep "Stored"

# 3. Count total telemetry points via API
TOTAL_POINTS=$(curl -s http://localhost:8080/api/v1/gpus | jq '[.[].id] | length')
echo "Total GPUs with telemetry: $TOTAL_POINTS"

# 4. Get detailed telemetry count for first GPU
FIRST_GPU=$(curl -s http://localhost:8080/api/v1/gpus | jq -r '.[0].id')
TELEMETRY_COUNT=$(curl -s "http://localhost:8080/api/v1/gpus/${FIRST_GPU}/telemetry" | jq 'length')
echo "Telemetry points for $FIRST_GPU: $TELEMETRY_COUNT"
```

### Step 9: Test Scalability

Test that the system handles multiple streamers and collectors:

```bash
# Scale up streamers
kubectl scale deployment gpu-telemetry-streamer --replicas=5

# Scale up collectors
kubectl scale deployment gpu-telemetry-collector --replicas=5

# Watch pods scale up
kubectl get pods -w

# Check that all replicas are running
kubectl get pods -l app.kubernetes.io/component=streamer
kubectl get pods -l app.kubernetes.io/component=collector

# Verify increased throughput in logs
kubectl logs -l app.kubernetes.io/component=streamer --tail=100 | grep "Published" | wc -l
kubectl logs -l app.kubernetes.io/component=collector --tail=100 | grep "Stored" | wc -l

# Scale back down
kubectl scale deployment gpu-telemetry-streamer --replicas=2
kubectl scale deployment gpu-telemetry-collector --replicas=2
```

### Step 10: Test Error Handling

Test graceful degradation:

```bash
# 1. Delete queue service temporarily
kubectl delete pod -l app.kubernetes.io/component=queue-service

# 2. Check that streamers handle connection errors gracefully
kubectl logs -l app.kubernetes.io/component=streamer --tail=20

# 3. Wait for queue service to restart
kubectl get pods -w

# 4. Verify system recovers automatically
curl http://localhost:8080/api/v1/gpus | jq
```

## Validation Checklist

Use this checklist to verify the system is working correctly:

- [ ] All pods are in `Running` state
- [ ] Streamers are reading CSV and publishing telemetry
- [ ] Queue service is receiving messages
- [ ] Collectors are consuming messages and storing telemetry
- [ ] API Gateway responds to health checks
- [ ] `GET /api/v1/gpus` returns list of GPUs
- [ ] `GET /api/v1/gpus/{id}/telemetry` returns telemetry data
- [ ] Time range filters work correctly
- [ ] Telemetry is ordered by timestamp (newest first)
- [ ] System scales with multiple replicas
- [ ] Swagger documentation is accessible

## Troubleshooting

### No Data Appearing

```bash
# Check if CSV ConfigMap exists and has data
kubectl get configmap gpu-telemetry-csv-data
kubectl describe configmap gpu-telemetry-csv-data | head -20

# Check streamer can read the CSV
kubectl exec -it deployment/gpu-telemetry-streamer -- ls -la /app/data/
kubectl exec -it deployment/gpu-telemetry-streamer -- cat /app/data/metrics.csv | head -5
```

### Pods Not Starting

```bash
# Check pod events
kubectl describe pod <pod-name>

# Common issues:
# - ImagePullBackOff: Images not loaded into kind
# - CrashLoopBackOff: Check logs with kubectl logs <pod-name>
# - ConfigMap not found: Create the CSV ConfigMap
```

### API Not Responding

```bash
# Check API Gateway pod is running
kubectl get pods -l app.kubernetes.io/component=api-gateway

# Check API Gateway logs for errors
kubectl logs -l app.kubernetes.io/component=api-gateway

# Verify port-forward is active
ps aux | grep "port-forward"

# Test from within cluster
kubectl run test-pod --rm -it --image=curlimages/curl -- sh
# Inside pod: curl http://gpu-telemetry-api-gateway:8080/health
```

## Cleanup

When you're done testing:

```bash
# Uninstall Helm release
helm uninstall gpu-telemetry

# Delete ConfigMap
kubectl delete configmap gpu-telemetry-csv-data

# Delete kind cluster
kind delete cluster --name gpu-telemetry
```

## Next Steps

After verifying the manual test:

1. Run the automated Go system test (see below)
2. Set up monitoring with Prometheus/Grafana
3. Implement persistent storage with MongoDB
4. Add horizontal pod autoscaling
5. Configure ingress for external access
