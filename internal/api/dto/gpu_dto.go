package dto

// GPUResponse represents a GPU in API responses
// Decouples internal domain.GPU from API contract
type GPUResponse struct {
	UUID      string `json:"uuid" example:"GPU-5fd4f087-86f3-7a43-b711-4771313afc50"`
	DeviceID  string `json:"device_id" example:"nvidia0"`
	GPUIndex  string `json:"gpu_index" example:"0"`
	ModelName string `json:"model_name" example:"NVIDIA H100 80GB HBM3"`
	Hostname  string `json:"hostname" example:"mtv5-dgx1-hgpu-031"`
	Container string `json:"container,omitempty" example:"gpu-workload"`
	Pod       string `json:"pod,omitempty" example:"training-pod-1"`
	Namespace string `json:"namespace,omitempty" example:"ml-team"`
}

// GPUListResponse wraps a list of GPUs
type GPUListResponse struct {
	GPUs  []*GPUResponse `json:"gpus"`
	Total int            `json:"total" example:"2"`
}
