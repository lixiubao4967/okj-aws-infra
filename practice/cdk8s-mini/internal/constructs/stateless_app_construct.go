// Package constructs contains cdk8s constructs for Kubernetes manifest generation.
package constructs

import (
	"cdk8s-mini/internal/spec"
	"log"
	"strconv"
	"strings"

	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
	cdk8splus "github.com/cdk8s-team/cdk8s-plus-go/cdk8splus33/v2"
)

// StatelessAppConstructProps defines what the caller must provide.
// Keep it focused: Name, Namespace, Image, Port are required. Everything else is optional.
type StatelessAppConstructProps struct {
	// Name is the service identifier — used as metadata.name for all K8s resources.
	Name string
	// Namespace is the Kubernetes namespace (e.g. "default", "okj-exchange").
	Namespace string
	// Image is the container image (e.g. "nginx:1.27-alpine").
	Image string
	// Replicas is the desired pod count. nil → construct default (1).
	Replicas *int
	// Port is the container port the application listens on (e.g. 80, 8080).
	Port int
	// Resources sets CPU/memory requests and limits. nil → no resource constraints.
	Resources *spec.ResourceRequirements
}

// StatelessAppConstruct generates a Kubernetes Deployment and a matching ClusterIP Service.
type StatelessAppConstruct struct {
	constructs.Construct
}

// NewStatelessAppConstruct creates the Deployment + Service for a stateless workload.
func NewStatelessAppConstruct(scope constructs.Construct, id string, props *StatelessAppConstructProps) *StatelessAppConstruct {
	validateProps(props)

	c := &StatelessAppConstruct{}
	c.Construct = constructs.NewConstruct(scope, jsii.String(id))

	// Determine replica count (default: 1 when Replicas is nil)
	replicas := float64(1)
	if props.Replicas != nil {
		replicas = float64(*props.Replicas)
	}

	// Stable pod-selector labels — must not change between synths.
	// cdk8s-plus would normally generate a content-hash label here, which changes
	// every synth and causes "field is immutable" errors on existing Deployments.
	// We use a fixed app.kubernetes.io/name label instead.
	appLabel := map[string]*string{
		"app.kubernetes.io/name":       jsii.String(props.Name),
		"app.kubernetes.io/managed-by": jsii.String("cdk8s"),
	}

	// ── Deployment ──────────────────────────────────────────────────────────────
	deployment := cdk8splus.NewDeployment(c.Construct, jsii.String("deployment"), &cdk8splus.DeploymentProps{
		Metadata: &cdk8s.ApiObjectMetadata{
			Name:      jsii.String(props.Name),
			Namespace: jsii.String(props.Namespace),
			Labels:    &appLabel,
		},
		Replicas: &replicas,
		PodMetadata: &cdk8s.ApiObjectMetadata{
			Labels: &appLabel,
		},
		Containers: &[]*cdk8splus.ContainerProps{
			{
				Name:      jsii.String(props.Name),
				Image:     jsii.String(props.Image),
				Ports:     &[]*cdk8splus.ContainerPort{{Number: jsii.Number(float64(props.Port))}},
				Resources: buildContainerResources(props.Resources),
			},
		},
	})

	// Patch selector/matchLabels and pod template labels to our stable set.
	// Must be done after NewDeployment because cdk8s-plus only exposes selector
	// through the Deployment object (no direct API to set it before creation).
	stableLabels := map[string]any{
		"app.kubernetes.io/name": props.Name,
	}
	deployObj := cdk8s.ApiObject_Of(deployment)
	deployObj.AddJsonPatch(cdk8s.JsonPatch_Replace(jsii.String("/spec/selector/matchLabels"), stableLabels))
	deployObj.AddJsonPatch(cdk8s.JsonPatch_Replace(jsii.String("/spec/template/metadata/labels"), stableLabels))

	// ── Service (ClusterIP) ──────────────────────────────────────────────────────
	service := cdk8splus.NewService(c.Construct, jsii.String("service"), &cdk8splus.ServiceProps{
		Metadata: &cdk8s.ApiObjectMetadata{
			Name:      jsii.String(props.Name),
			Namespace: jsii.String(props.Namespace),
			Labels:    &appLabel,
		},
		Ports: &[]*cdk8splus.ServicePort{
			{
				Port:       jsii.Number(float64(props.Port)),
				TargetPort: jsii.Number(float64(props.Port)),
			},
		},
		Selector: deployment,
	})

	// Same fix: replace the cdk8s-plus generated selector with our stable label.
	svcObj := cdk8s.ApiObject_Of(service)
	svcObj.AddJsonPatch(cdk8s.JsonPatch_Replace(jsii.String("/spec/selector"), stableLabels))

	return c
}

// ── Validation ───────────────────────────────────────────────────────────────

func validateProps(props *StatelessAppConstructProps) {
	if props == nil {
		log.Panicf("StatelessAppConstructProps cannot be nil")
	}
	if props.Name == "" {
		log.Panicf("StatelessAppConstructProps.Name is required")
	}
	if props.Namespace == "" {
		log.Panicf("StatelessAppConstructProps.Namespace is required")
	}
	if props.Image == "" {
		log.Panicf("StatelessAppConstructProps.Image is required")
	}
	if props.Port == 0 {
		log.Panicf("StatelessAppConstructProps.Port is required")
	}
}

// ── Resource helpers ─────────────────────────────────────────────────────────

// buildContainerResources converts ResourceRequirements to cdk8s-plus typed quantities.
// Returns nil when resources is nil (no resource constraints set).
func buildContainerResources(resources *spec.ResourceRequirements) *cdk8splus.ContainerResources {
	if resources == nil {
		return nil
	}
	return &cdk8splus.ContainerResources{
		Cpu: &cdk8splus.CpuResources{
			Request: cdk8splus.Cpu_Units(jsii.Number(parseCPU(resources.CPURequest))),
			Limit:   cdk8splus.Cpu_Units(jsii.Number(parseCPU(resources.CPULimit))),
		},
		Memory: &cdk8splus.MemoryResources{
			Request: cdk8s.Size_Mebibytes(jsii.Number(parseMemory(resources.MemoryRequest))),
			Limit:   cdk8s.Size_Mebibytes(jsii.Number(parseMemory(resources.MemoryLimit))),
		},
	}
}

// parseCPU parses a Kubernetes CPU string and returns cores as float64.
// "100m" → 0.1, "0.5" → 0.5, "1" → 1.0
func parseCPU(cpu string) float64 {
	if cpu == "" {
		return 0
	}
	if before, found := strings.CutSuffix(cpu, "m"); found {
		val, err := strconv.ParseFloat(before, 64)
		if err != nil {
			log.Panicf("invalid CPU value: %s", cpu)
		}
		return val / 1000
	}
	val, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		log.Panicf("invalid CPU value: %s", cpu)
	}
	return val
}

// parseMemory parses a Kubernetes memory string and returns mebibytes as float64.
// "256Mi" → 256, "1Gi" → 1024, "512M" → ~488
func parseMemory(mem string) float64 {
	if mem == "" {
		return 0
	}
	type binarySuffix struct {
		suffix string
		factor float64
	}
	for _, s := range []binarySuffix{
		{"Pi", 1024 * 1024 * 1024},
		{"Ti", 1024 * 1024},
		{"Gi", 1024},
		{"Mi", 1},
		{"Ki", 1.0 / 1024},
	} {
		if before, found := strings.CutSuffix(mem, s.suffix); found {
			val, err := strconv.ParseFloat(before, 64)
			if err != nil {
				log.Panicf("invalid memory value: %s", mem)
			}
			return val * s.factor
		}
	}
	const mibytes = 1024 * 1024
	type siSuffix struct {
		suffix string
		factor float64
	}
	for _, s := range []siSuffix{
		{"G", 1e9 / mibytes},
		{"M", 1e6 / mibytes},
		{"K", 1e3 / mibytes},
	} {
		if before, found := strings.CutSuffix(mem, s.suffix); found {
			val, err := strconv.ParseFloat(before, 64)
			if err != nil {
				log.Panicf("invalid memory value: %s", mem)
			}
			return val * s.factor
		}
	}
	val, err := strconv.ParseFloat(mem, 64)
	if err != nil {
		log.Panicf("invalid memory value: %s", mem)
	}
	return val / mibytes
}
