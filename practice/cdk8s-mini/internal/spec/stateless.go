package spec

// StatelessAppSpec is implemented by services backed by StatelessAppConstruct (Deployment + Service).
// Embed BaseStatelessAppSpec and override only the methods that deviate from defaults.
type StatelessAppSpec interface {
	Name() string
	Replicas() *int
	Resources() *ResourceRequirements
	// ReadOnlyRootFilesystem controls whether the container root FS is read-only.
	// nil → cdk8s-plus default (true). Return ptr.To(false) if the service needs
	// to write to paths outside mounted volumes (e.g. /var/cache, /tmp).
	ReadOnlyRootFilesystem() *bool
}
