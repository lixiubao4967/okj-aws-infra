// Package spec defines service specification interfaces and default implementations.
// No imports from other internal packages — prevents import cycles.
package spec

// BaseServiceSpec provides default implementations for service identity methods.
// Embed this in any chart struct and set ServiceName in the constructor.
type BaseServiceSpec struct {
	ServiceName string
}

// Name returns the canonical service identifier (used as chart name and K8s resource name).
func (b BaseServiceSpec) Name() string { return b.ServiceName }

// Replicas returns nil — "no opinion, defer to construct default (1)".
// Override to return a fixed replica count.
func (b BaseServiceSpec) Replicas() *int { return nil }

// Resources returns nil — "no resource limits, let Kubernetes use node defaults".
// Override to set CPU/memory requests and limits.
func (b BaseServiceSpec) Resources() *ResourceRequirements { return nil }

// BaseStatelessAppSpec embeds BaseServiceSpec and provides full StatelessAppSpec defaults.
// Embed this in service chart structs and override only what deviates from defaults.
type BaseStatelessAppSpec struct {
	BaseServiceSpec
}
