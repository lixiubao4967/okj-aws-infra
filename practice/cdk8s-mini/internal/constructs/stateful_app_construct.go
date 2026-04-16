package constructs

// StatefulApp vs StatelessApp differences:
//
//	StatelessApp (Deployment + ClusterIP Service)
//	  └─ Pods are interchangeable; any pod can be replaced without data loss.
//
//	StatefulApp (StatefulSet + Headless Service + volumeClaimTemplates)
//	  ├─ Pods have stable names: redis-0, redis-1, ...
//	  ├─ Headless Service (ClusterIP: None) enables DNS:
//	  │     redis-0.redis-stateful.default.svc.cluster.local
//	  └─ Each pod gets its own PVC — data persists across restarts/reschedules.
//
// ⚠  Known limitation (cdk8s Go JSII binding v2.70):
// PVC storage size always renders as "null" in volumeClaimTemplates because the
// Go JSII bridge cannot serialize any K8s Quantity type (plain string, cdk8s.Size,
// k8s.Quantity) inside JsonPatch operations. The cdk8s-plus TypeScript binding
// does NOT have this issue. Workaround: after `go run cmd/main.go`, manually set
// `storage: 1Gi` (or the desired size) in the generated YAML before applying.

import (
	"cdk8s-mini/internal/spec"
	"log"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
	cdk8splus "github.com/cdk8s-team/cdk8s-plus-go/cdk8splus33/v2"
)

// StatefulAppConstructProps defines what the caller must provide.
type StatefulAppConstructProps struct {
	Name      string
	Namespace string
	Image     string
	Replicas  *int
	Port      int
	Resources *spec.ResourceRequirements
	// ReadOnlyRootFilesystem overrides cdk8s-plus default (true). Most stateful
	// services need ptr.To(false) because they write outside their data volume.
	ReadOnlyRootFilesystem *bool
	// StorageSize is the PVC capacity per pod (e.g. "1Gi", "500Mi").
	// Note: due to the Go JSII Quantity serialization bug, this value is stored
	// as metadata only — the generated YAML will show "storage: null" and must
	// be patched manually before applying to a cluster.
	StorageSize string
	// MountPath is where the PVC is mounted inside the container (e.g. "/data").
	MountPath string
}

// StatefulAppConstruct generates a StatefulSet + Headless Service + volumeClaimTemplates.
type StatefulAppConstruct struct {
	constructs.Construct
}

// NewStatefulAppConstruct creates the StatefulSet + Headless Service for a stateful workload.
func NewStatefulAppConstruct(scope constructs.Construct, id string, props *StatefulAppConstructProps) *StatefulAppConstruct {
	validateStatefulProps(props)

	c := &StatefulAppConstruct{}
	c.Construct = constructs.NewConstruct(scope, jsii.String(id))

	replicas := float64(1)
	if props.Replicas != nil {
		replicas = float64(*props.Replicas)
	}

	appLabel := map[string]*string{
		"app.kubernetes.io/name":       jsii.String(props.Name),
		"app.kubernetes.io/managed-by": jsii.String("cdk8s"),
	}
	stableLabels := map[string]any{
		"app.kubernetes.io/name": props.Name,
	}

	// ── Headless Service ─────────────────────────────────────────────────────────
	// ClusterIP: None → DNS-based pod discovery instead of virtual IP load-balancing.
	// Pods are reachable at: <pod-name>.<svc-name>.<namespace>.svc.cluster.local
	headlessSvc := cdk8splus.NewService(c.Construct, jsii.String("headless-service"), &cdk8splus.ServiceProps{
		Metadata: &cdk8s.ApiObjectMetadata{
			Name:      jsii.String(props.Name),
			Namespace: jsii.String(props.Namespace),
			Labels:    &appLabel,
		},
		ClusterIP: jsii.String("None"),
		Ports: &[]*cdk8splus.ServicePort{
			{
				Port:       jsii.Number(float64(props.Port)),
				TargetPort: jsii.Number(float64(props.Port)),
			},
		},
	})
	svcObj := cdk8s.ApiObject_Of(headlessSvc)
	svcObj.AddJsonPatch(cdk8s.JsonPatch_Add(jsii.String("/spec/selector"), stableLabels))

	// ── Security context override ────────────────────────────────────────────────
	var secCtx *cdk8splus.ContainerSecurityContextProps
	if props.ReadOnlyRootFilesystem != nil {
		secCtx = &cdk8splus.ContainerSecurityContextProps{
			ReadOnlyRootFilesystem: jsii.Bool(*props.ReadOnlyRootFilesystem),
		}
	}

	// ── Volume placeholder for cdk8s-plus validation ─────────────────────────────
	// cdk8s-plus validates at Synth() that every VolumeClaimTemplate name is
	// referenced by a container volumeMount. ContainerProps.VolumeMounts requires a
	// Volume object — Volume_FromName creates a placeholder by name. The spurious
	// spec.volumes entry it generates is removed via JsonPatch_Remove after creation.
	dataVolume := cdk8splus.Volume_FromName(c.Construct, jsii.String("data-volume-ref"), jsii.String("data"))

	// ── StatefulSet ──────────────────────────────────────────────────────────────
	// PersistentVolumeClaimTemplateProps.Name (top-level, required) names the claim;
	// format of created PVCs: <Name>-<pod-name>  →  data-redis-stateful-0, ...
	statefulSet := cdk8splus.NewStatefulSet(c.Construct, jsii.String("statefulset"), &cdk8splus.StatefulSetProps{
		Metadata: &cdk8s.ApiObjectMetadata{
			Name:      jsii.String(props.Name),
			Namespace: jsii.String(props.Namespace),
			Labels:    &appLabel,
		},
		Service:  headlessSvc,
		Replicas: &replicas,
		PodMetadata: &cdk8s.ApiObjectMetadata{
			Labels: &appLabel,
		},
		Containers: &[]*cdk8splus.ContainerProps{
			{
				Name:            jsii.String(props.Name),
				Image:           jsii.String(props.Image),
				Ports:           &[]*cdk8splus.ContainerPort{{Number: jsii.Number(float64(props.Port))}},
				Resources:       buildContainerResources(props.Resources),
				SecurityContext: secCtx,
				VolumeMounts: &[]*cdk8splus.VolumeMount{
					{Volume: dataVolume, Path: jsii.String(props.MountPath)},
				},
			},
		},
		VolumeClaimTemplates: &[]*cdk8splus.PersistentVolumeClaimTemplateProps{
			{
				Name:        jsii.String("data"), // required top-level field (not Metadata.Name)
				AccessModes: &[]cdk8splus.PersistentVolumeAccessMode{cdk8splus.PersistentVolumeAccessMode_READ_WRITE_ONCE},
				// Storage omitted: cdk8s Go JSII binding serializes ALL Quantity types
				// (cdk8s.Size, k8s.Quantity, plain string) to null in this field.
				// The generated YAML will have "storage: null" — fix manually before deploy.
			},
		},
	})

	statefulSetObj := cdk8s.ApiObject_Of(statefulSet)

	// Patch selector to stable labels (same "field is immutable" fix as Deployment)
	statefulSetObj.AddJsonPatch(cdk8s.JsonPatch_Replace(jsii.String("/spec/selector/matchLabels"), stableLabels))
	statefulSetObj.AddJsonPatch(cdk8s.JsonPatch_Replace(jsii.String("/spec/template/metadata/labels"), stableLabels))

	// Remove the spurious spec.volumes entry added by Volume_FromName.
	// In a StatefulSet, the data volume comes from volumeClaimTemplates, not spec.volumes.
	statefulSetObj.AddJsonPatch(cdk8s.JsonPatch_Remove(jsii.String("/spec/template/spec/volumes")))

	return c
}

// ── Validation ───────────────────────────────────────────────────────────────

func validateStatefulProps(props *StatefulAppConstructProps) {
	if props == nil {
		log.Panicf("StatefulAppConstructProps cannot be nil")
	}
	if props.Name == "" {
		log.Panicf("StatefulAppConstructProps.Name is required")
	}
	if props.Namespace == "" {
		log.Panicf("StatefulAppConstructProps.Namespace is required")
	}
	if props.Image == "" {
		log.Panicf("StatefulAppConstructProps.Image is required")
	}
	if props.Port == 0 {
		log.Panicf("StatefulAppConstructProps.Port is required")
	}
	if props.StorageSize == "" {
		log.Panicf("StatefulAppConstructProps.StorageSize is required (e.g. \"1Gi\")")
	}
	if props.MountPath == "" {
		log.Panicf("StatefulAppConstructProps.MountPath is required (e.g. \"/data\")")
	}
}
