# Kubernetes Deployment Guide with kind and Helm

This guide shows how to deploy the GPU Telemetry Pipeline on a local Kubernetes cluster using **kind** (Kubernetes in Docker) and **Helm**.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) installed and running
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) installed
- [kubectl](https://kubernetes.io/docs/tasks/tools/) installed
- [Helm](https://helm.sh/docs/intro/install/) v3.x installed

## Quick Start

### 1. Create a kind Cluster

```bash
# Create a kind cluster named "gpu-telemetry"
kind create cluster --name gpu-telemetry

# Verify the cluster is running
kubectl cluster-info --context kind-gpu-telemetry
kubectl get nodes
```

### 2. Build Docker Images

Build all service Docker images:

```bash
# Build all images at once
make docker-build

# Or build individually:
make docker-streamer
make docker-collector
make docker-api
make docker-queue
```

This creates the following images:
- `gpu-telemetry-streamer:latest`
- `gpu-telemetry-collector:latest`
- `gpu-telemetry-api:latest`
- `gpu-telemetry-queue:latest`

### 3. Load Images into kind

Load the Docker images into the kind cluster:

```bash
kind load docker-image gpu-telemetry-streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-api:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-queue:latest --name gpu-telemetry
```

### 4. Create CSV ConfigMap

The streamer needs the CSV data to be available. Create a ConfigMap from the CSV file:

```bash
kubectl create configmap gpu-telemetry-csv-data \
  --from-file=metrics.csv=data/dcgm_metrics_20250718_134233.csv
```

**Note**: The ConfigMap name must match the one expected by the Helm chart. If you're using a custom release name, adjust accordingly.

### 5. Install the Helm Chart

Install the GPU Telemetry Pipeline using Helm:

```bash
# Install with default values
helm install gpu-telemetry ./charts/gpu-telemetry

# Or install with custom values
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.replicaCount=3 \
  --set collector.replicaCount=3
```

### 6. Verify the Deployment

Check that all pods are running:

```bash
# Watch pod status
kubectl get pods -w

# Expected output (after a few seconds):
# NAME                                              READY   STATUS    RESTARTS   AGE
# gpu-telemetry-streamer-xxxxxxxxxx-xxxxx          1/1     Running   0          30s
# gpu-telemetry-streamer-xxxxxxxxxx-xxxxx          1/1     Running   0          30s
# gpu-telemetry-collector-xxxxxxxxxx-xxxxx         1/1     Running   0          30s
# gpu-telemetry-collector-xxxxxxxxxx-xxxxx         1/1     Running   0          30s
# gpu-telemetry-api-gateway-xxxxxxxxxx-xxxxx       1/1     Running   0          30s
# gpu-telemetry-queue-service-xxxxxxxxxx-xxxxx     1/1     Running   0          30s

# Check services
kubectl get svc

# Check logs
kubectl logs -l app.kubernetes.io/component=streamer
kubectl logs -l app.kubernetes.io/component=collector
kubectl logs -l app.kubernetes.io/component=api-gateway
```

### 7. Access the API Gateway

The API Gateway is exposed as a ClusterIP service. Use port-forwarding to access it:

```bash
# Forward port 8080 from the API Gateway service
kubectl port-forward service/gpu-telemetry-api-gateway 8080:8080
```

Now you can access the API at `http://localhost:8080`:

```bash
# Test the health endpoint
curl http://localhost:8080/health

# Get all GPUs
curl http://localhost:8080/api/v1/gpus

# Get telemetry for a specific GPU
curl "http://localhost:8080/api/v1/gpus/<gpu-id>/telemetry"

# Get telemetry with time filters
curl "http://localhost:8080/api/v1/gpus/<gpu-id>/telemetry?start_time=2025-01-01T00:00:00Z&end_time=2025-12-31T23:59:59Z"

# View Swagger documentation
open http://localhost:8080/swagger/index.html
```

## Helm Commands Reference

### Install/Upgrade

```bash
# Install the chart
helm install gpu-telemetry ./charts/gpu-telemetry

# Install with custom release name
helm install my-gpu-pipeline ./charts/gpu-telemetry

# Upgrade an existing release
helm upgrade gpu-telemetry ./charts/gpu-telemetry

# Install or upgrade (idempotent)
helm upgrade --install gpu-telemetry ./charts/gpu-telemetry
```

### Customization

```bash
# Override replica counts
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.replicaCount=5 \
  --set collector.replicaCount=5

# Use a custom values file
helm install gpu-telemetry ./charts/gpu-telemetry \
  --values custom-values.yaml

# Override image tags
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.image.tag=v1.0.0 \
  --set collector.image.tag=v1.0.0

# Set log level
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.env.logLevel=debug \
  --set collector.env.logLevel=debug
```

### Management

```bash
# List releases
helm list

# Get release status
helm status gpu-telemetry

# Get release values
helm get values gpu-telemetry

# Get manifest
helm get manifest gpu-telemetry

# Uninstall the release
helm uninstall gpu-telemetry

# Dry run (preview what will be installed)
helm install gpu-telemetry ./charts/gpu-telemetry --dry-run --debug
```

## Scaling

Scale the streamers and collectors:

```bash
# Scale using kubectl
kubectl scale deployment gpu-telemetry-streamer --replicas=5
kubectl scale deployment gpu-telemetry-collector --replicas=5

# Or upgrade with Helm
helm upgrade gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.replicaCount=5 \
  --set collector.replicaCount=5
```

## Troubleshooting

### Check Pod Logs

```bash
# View logs for a specific component
kubectl logs -l app.kubernetes.io/component=streamer --tail=100
kubectl logs -l app.kubernetes.io/component=collector --tail=100
kubectl logs -l app.kubernetes.io/component=api-gateway --tail=100
kubectl logs -l app.kubernetes.io/component=queue-service --tail=100

# Follow logs in real-time
kubectl logs -l app.kubernetes.io/component=streamer -f

# View logs for a specific pod
kubectl logs <pod-name>
```

### Debug Pod Issues

```bash
# Describe a pod to see events
kubectl describe pod <pod-name>

# Get pod YAML
kubectl get pod <pod-name> -o yaml

# Execute commands in a pod
kubectl exec -it <pod-name> -- /bin/sh

# Check resource usage
kubectl top pods
kubectl top nodes
```

### Common Issues

#### Images Not Found

If pods show `ImagePullBackOff` or `ErrImagePull`:

```bash
# Reload images into kind
kind load docker-image gpu-telemetry-streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-api:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-queue:latest --name gpu-telemetry

# Restart pods
kubectl rollout restart deployment gpu-telemetry-streamer
kubectl rollout restart deployment gpu-telemetry-collector
kubectl rollout restart deployment gpu-telemetry-api-gateway
kubectl rollout restart deployment gpu-telemetry-queue-service
```

#### CSV ConfigMap Missing

```bash
# Check if ConfigMap exists
kubectl get configmap

# Recreate ConfigMap
kubectl delete configmap gpu-telemetry-csv-data
kubectl create configmap gpu-telemetry-csv-data \
  --from-file=metrics.csv="../dcgm_metrics_20250718_134233 (1)(1).csv"

# Restart streamer pods to pick up the ConfigMap
kubectl rollout restart deployment gpu-telemetry-streamer
```

#### Service Not Accessible

```bash
# Check service endpoints
kubectl get endpoints

# Verify service selector matches pod labels
kubectl get svc gpu-telemetry-api-gateway -o yaml
kubectl get pods -l app.kubernetes.io/component=api-gateway --show-labels

# Test from within the cluster
kubectl run test-pod --rm -it --image=curlimages/curl -- sh
# Inside the pod:
curl http://gpu-telemetry-api-gateway:8080/health
```

## Advanced Configuration

### Using Custom Values

Create a `custom-values.yaml`:

```yaml
streamer:
  replicaCount: 3
  env:
    streamInterval: "500ms"
    logLevel: "debug"

collector:
  replicaCount: 3
  env:
    batchSize: "20"
    logLevel: "debug"

apiGateway:
  service:
    type: NodePort  # Change to NodePort for external access
```

Install with custom values:

```bash
helm install gpu-telemetry ./charts/gpu-telemetry -f custom-values.yaml
```

### Resource Limits

Adjust resource limits in `values.yaml` or via command line:

```bash
helm install gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.resources.limits.cpu=1000m \
  --set streamer.resources.limits.memory=1Gi
```

## Cleanup

### Delete the Helm Release

```bash
helm uninstall gpu-telemetry
```

### Delete the ConfigMap

```bash
kubectl delete configmap gpu-telemetry-csv-data
```

### Delete the kind Cluster

```bash
kind delete cluster --name gpu-telemetry
```

## Next Steps

- Monitor metrics and logs using Prometheus/Grafana
- Set up horizontal pod autoscaling (HPA)
- Implement persistent storage for telemetry data
- Add Ingress for external access
- Configure RBAC and network policies
- Integrate with MongoDB for persistent storage

## Production Considerations

When moving to production:

1. **Image Registry**: Push images to a container registry (Docker Hub, GCR, ECR, etc.)
2. **Image Tags**: Use specific version tags instead of `latest`
3. **ConfigMap Size**: For large CSV files, use PersistentVolumes instead of ConfigMaps
4. **Storage**: Replace in-memory storage with MongoDB or another persistent database
5. **Monitoring**: Add Prometheus metrics and Grafana dashboards
6. **High Availability**: Increase replica counts and configure pod disruption budgets
7. **Security**: Implement network policies, pod security policies, and service accounts
8. **Resource Limits**: Set appropriate CPU and memory limits based on load testing
9. **Autoscaling**: Configure HPA based on CPU/memory or custom metrics
10. **Backup**: Implement backup strategies for persistent data
