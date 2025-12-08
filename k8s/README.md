# GPU Telemetry Pipeline - Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the GPU Telemetry Pipeline.

## Architecture

```
┌─────────────────────────────────────────────┐
│          Queue Service (StatefulSet)         │
│              1 replica                       │
│   In-memory competing consumer queue         │
│   Exposes HTTP API on port 8080             │
└─────────────────────────────────────────────┘
              ▲                    ▲
              │                    │
      ┌───────┴──────┐    ┌───────┴────────┐
      │   Streamer   │    │   Collector     │
      │  (Deployment)│    │   (Deployment)  │
      │  N replicas  │    │   N replicas    │
      └──────────────┘    └─────────────────┘
```

## Services

### 1. Queue Service (StatefulSet)
- **Replicas**: 1 (single instance)
- **Type**: StatefulSet (for stable network identity)
- **Image**: `gpu-metrics-streamer/queueservice:latest`
- **Port**: 8080
- **Resources**: 256Mi-512Mi RAM, 250m-500m CPU
- **Endpoints**:
  - `POST /api/v1/queue/publish` - Publish messages
  - `GET /api/v1/queue/subscribe` - Subscribe via SSE
  - `POST /api/v1/queue/ack` - Acknowledge message
  - `POST /api/v1/queue/nack` - Requeue message
  - `GET /api/v1/queue/stats` - Queue statistics
  - `GET /health` - Health check

### 2. Streamer (Deployment)
- **Replicas**: 2 (scalable)
- **Type**: Deployment
- **Image**: `gpu-metrics-streamer/streamer:latest`
- **Resources**: 128Mi-256Mi RAM, 100m-200m CPU
- **Environment**:
  - `QUEUE_SERVICE_URL=http://queue-service:8080`
  - `CSV_PATH=/data/dcgm_metrics.csv`
  - `STREAM_INTERVAL=100ms`
  - `LOOP_MODE=true`

### 3. Collector (Deployment)
- **Replicas**: 3 (scalable)
- **Type**: Deployment
- **Image**: `gpu-metrics-streamer/collector:latest`
- **Resources**: 128Mi-256Mi RAM, 100m-200m CPU
- **Environment**:
  - `QUEUE_SERVICE_URL=http://queue-service:8080`
  - `BATCH_SIZE=1`
  - `MAX_CONCURRENT_HANDLERS=10`
  - `MONGODB_URI` (from secret)

## Deployment

### Prerequisites
1. Kubernetes cluster (v1.19+)
2. kubectl configured
3. Docker images built and pushed

### Build Docker Images

```bash
# Build images
docker build -t gpu-metrics-streamer/queueservice:latest -f Dockerfile.queueservice .
docker build -t gpu-metrics-streamer/streamer:latest -f Dockerfile.streamer .
docker build -t gpu-metrics-streamer/collector:latest -f Dockerfile.collector .

# Push to registry (if using remote cluster)
docker push gpu-metrics-streamer/queueservice:latest
docker push gpu-metrics-streamer/streamer:latest
docker push gpu-metrics-streamer/collector:latest
```

### Deploy to Kubernetes

```bash
# Deploy queue service first
kubectl apply -f k8s/queue-service.yaml

# Wait for queue service to be ready
kubectl wait --for=condition=ready pod -l app=queue-service --timeout=60s

# Deploy streamer
kubectl apply -f k8s/streamer.yaml

# Deploy collector
kubectl apply -f k8s/collector.yaml
```

### Verify Deployment

```bash
# Check all pods are running
kubectl get pods

# Check queue service
kubectl get svc queue-service

# Check logs
kubectl logs -l app=queue-service
kubectl logs -l app=streamer
kubectl logs -l app=collector

# Port forward to access queue API
kubectl port-forward svc/queue-service 8080:8080

# Check queue stats
curl http://localhost:8080/api/v1/queue/stats | jq .

# Check health
curl http://localhost:8080/health
```

## Scaling

### Scale Streamers
```bash
kubectl scale deployment streamer --replicas=5
```

### Scale Collectors
```bash
kubectl scale deployment collector --replicas=10
```

**Note**: Queue service should remain at 1 replica (in-memory queue limitation).

## Monitoring

### Queue Statistics
```bash
kubectl port-forward svc/queue-service 8080:8080
curl http://localhost:8080/api/v1/queue/stats
```

### Pod Metrics
```bash
kubectl top pods
```

### Logs
```bash
# Follow queue service logs
kubectl logs -f -l app=queue-service

# Follow streamer logs
kubectl logs -f -l app=streamer

# Follow collector logs (all replicas)
kubectl logs -f -l app=collector --max-log-requests=10
```

## Troubleshooting

### Collectors not receiving messages
```bash
# Check queue service logs
kubectl logs -l app=queue-service | grep -i consumer

# Check collector connection
kubectl logs -l app=collector | grep -i "queue client"

# Verify service DNS
kubectl run -it --rm debug --image=busybox --restart=Never -- nslookup queue-service
```

### Streamer not publishing
```bash
# Check streamer logs
kubectl logs -l app=streamer | grep -i publish

# Check CSV data mounted
kubectl exec -it <streamer-pod> -- cat /data/dcgm_metrics.csv
```

### Queue service OOM
```bash
# Increase memory limit in queue-service.yaml
resources:
  limits:
    memory: "1Gi"

# Apply changes
kubectl apply -f k8s/queue-service.yaml
kubectl rollout restart statefulset/queue-service
```

## Configuration

### Update Queue Settings
Edit `k8s/queue-service.yaml`:
```yaml
env:
- name: VISIBILITY_TIMEOUT
  value: "10m"  # Increase for slow collectors
- name: MAX_RETRIES
  value: "5"    # More retries before dead letter
- name: BUFFER_SIZE
  value: "5000" # Larger buffer for high throughput
```

### Update Streamer Settings
Edit `k8s/streamer.yaml`:
```yaml
env:
- name: STREAM_INTERVAL
  value: "50ms"   # Faster publishing
- name: LOOP_MODE
  value: "false"  # One-time run
```

### Update Collector Settings
Edit `k8s/collector.yaml`:
```yaml
env:
- name: MAX_CONCURRENT_HANDLERS
  value: "20"  # More concurrent processing
```

## Clean Up

```bash
# Delete all resources
kubectl delete -f k8s/collector.yaml
kubectl delete -f k8s/streamer.yaml
kubectl delete -f k8s/queue-service.yaml
```

## Production Considerations

1. **High Availability**: Current queue service is single-replica. For HA:
   - Implement Redis/MongoDB-based queue
   - Deploy queue service with 3+ replicas

2. **Persistence**: In-memory queue loses messages on pod restart
   - Implement persistent queue backend
   - Or accept message loss on restarts

3. **Resource Limits**: Adjust based on load testing
   - Monitor memory usage of queue service
   - Scale collectors based on queue depth

4. **Security**:
   - Add Network Policies
   - Use TLS for queue service API
   - Rotate MongoDB credentials

5. **Monitoring**:
   - Add Prometheus metrics
   - Set up Grafana dashboards
   - Configure alerts for queue depth

## Next Steps

1. Create Dockerfiles for each service
2. Set up CI/CD pipeline
3. Implement Helm charts for easier deployment
4. Add horizontal pod autoscaling
5. Integrate with existing monitoring stack
