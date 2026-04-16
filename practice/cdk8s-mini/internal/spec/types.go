package spec

// ResourceRequirements defines CPU and memory resource requests and limits for a container.
type ResourceRequirements struct {
	// CPURequest is the CPU resource request (e.g., "100m", "0.5", "1").
	CPURequest string
	// CPULimit is the CPU resource limit (e.g., "500m", "1", "2").
	CPULimit string
	// MemoryRequest is the memory resource request (e.g., "128Mi", "1Gi").
	MemoryRequest string
	// MemoryLimit is the memory resource limit (e.g., "256Mi", "2Gi").
	MemoryLimit string
}
