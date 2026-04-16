package cache

import (
	"cdk8s-mini/internal/constructs"
	"cdk8s-mini/internal/ptr"
	"cdk8s-mini/internal/spec"

	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

// RedisStatefulChart is the StatefulApp counterpart of RedisChart.
//
// Use this when Redis must survive pod restarts with persistent data (production-like).
// Use RedisChart (Deployment) for ephemeral caches where data loss is acceptable.
type RedisStatefulChart struct {
	spec.BaseStatelessAppSpec
}

func NewRedisStatefulChart() RedisStatefulChart {
	return RedisStatefulChart{
		BaseStatelessAppSpec: spec.BaseStatelessAppSpec{
			BaseServiceSpec: spec.BaseServiceSpec{ServiceName: "redis-stateful"},
		},
	}
}

// ReadOnlyRootFilesystem returns false — Redis writes lock files to /tmp even
// when /data is a separate PVC mount.
func (c RedisStatefulChart) ReadOnlyRootFilesystem() *bool { return ptr.To(false) }

func (c RedisStatefulChart) Resources() *spec.ResourceRequirements {
	return &spec.ResourceRequirements{
		CPURequest:    "100m",
		CPULimit:      "200m",
		MemoryRequest: "128Mi",
		MemoryLimit:   "256Mi",
	}
}

// StorageSize returns the PVC capacity per pod.
func (c RedisStatefulChart) StorageSize() string { return "1Gi" }

// MountPath returns the directory where Redis stores its data files.
func (c RedisStatefulChart) MountPath() string { return "/data" }

func (c RedisStatefulChart) BuildFunc() func(cdk8s.App) {
	return func(app cdk8s.App) {
		chart := cdk8s.NewChart(app, jsii.String(c.Name()), &cdk8s.ChartProps{
			Namespace: jsii.String("default"),
		})
		constructs.NewStatefulAppConstruct(chart, c.Name(), &constructs.StatefulAppConstructProps{
			Name:                   c.Name(),
			Namespace:              "default",
			Image:                  "redis:7-alpine",
			Replicas:               c.Replicas(),
			Port:                   6379,
			Resources:              c.Resources(),
			ReadOnlyRootFilesystem: c.ReadOnlyRootFilesystem(),
			StorageSize:            c.StorageSize(),
			MountPath:              c.MountPath(),
		})
	}
}
