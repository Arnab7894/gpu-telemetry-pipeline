package domain

// GPU represents a physical GPU device in the cluster
// Maps to CSV columns: uuid, device, gpu_id, modelName, Hostname, container, pod, namespace
type GPU struct {
	// UUID is the unique hardware identifier for the GPU (CSV: uuid)
	// This is the PRIMARY KEY used in API endpoints: /api/v1/gpus/{uuid}
	// Example: "GPU-5fd4f087-86f3-7a43-b711-4771313afc50"
	UUID string `json:"uuid"`

	// DeviceID is the device identifier (CSV: device)
	// Example: "nvidia0", "nvidia1"
	DeviceID string `json:"device_id"`

	// GPUIndex is the logical GPU index on the host (CSV: gpu_id)
	// Example: "0", "1", "2", etc.
	GPUIndex string `json:"gpu_index"`

	// ModelName is the GPU model (CSV: modelName)
	// Example: "NVIDIA H100 80GB HBM3"
	ModelName string `json:"model_name"`

	// Hostname is the host machine where the GPU resides (CSV: Hostname)
	// Example: "mtv5-dgx1-hgpu-031"
	Hostname string `json:"hostname"`

	// Container name (CSV: container) - optional, for Kubernetes deployments
	Container string `json:"container,omitempty"`

	// Pod name (CSV: pod) - optional, for Kubernetes deployments
	Pod string `json:"pod,omitempty"`

	// Namespace (CSV: namespace) - optional, for Kubernetes deployments
	Namespace string `json:"namespace,omitempty"`
}

// GetCompositeKey returns a composite key for the GPU
// Alternative to UUID: Hostname + DeviceID uniquely identifies a GPU
func (g *GPU) GetCompositeKey() string {
	return g.Hostname + ":" + g.DeviceID
}
