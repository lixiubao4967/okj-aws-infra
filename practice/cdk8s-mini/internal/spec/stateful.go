package spec

// StatefulAppSpec is implemented by services backed by StatefulAppConstruct.
//
// Key differences vs StatelessAppSpec:
//   - Uses StatefulSet → ordered pod creation, stable pod names (redis-0, redis-1)
//   - Governed by a headless Service (ClusterIP: None) → DNS: redis-0.<name>.default.svc.cluster.local
//   - Each pod gets its own PVC via volumeClaimTemplates → storage survives pod restarts
//
// Embed BaseStatelessAppSpec to inherit defaults for Name/Replicas/Resources/ReadOnlyRootFilesystem.
type StatefulAppSpec interface {
	Name() string
	Replicas() *int
	Resources() *ResourceRequirements
	ReadOnlyRootFilesystem() *bool
	// StorageSize is the PVC capacity per pod (e.g. "1Gi", "500Mi").
	StorageSize() string
	// MountPath is where the PVC is mounted inside the container (e.g. "/data").
	MountPath() string
}
