package spec

// StatelessAppSpec is implemented by services backed by StatelessAppConstruct (Deployment + Service).
// Embed BaseStatelessAppSpec and override only the methods that deviate from defaults.
type StatelessAppSpec interface {
	Name() string
	Replicas() *int
	Resources() *ResourceRequirements
}
