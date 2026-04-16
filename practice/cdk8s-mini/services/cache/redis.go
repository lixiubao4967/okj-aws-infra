// Package cache contains chart specifications for cache-tier services.
package cache

import (
	"cdk8s-mini/internal/constructs"
	"cdk8s-mini/internal/ptr"
	"cdk8s-mini/internal/spec"

	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

const redisName = "redis"

// RedisChart declares the requirements for the Redis deployment.
// Embed BaseStatelessAppSpec and override only what deviates from defaults.
type RedisChart struct {
	spec.BaseStatelessAppSpec
}

// NewRedisChart constructs a RedisChart with the service name set.
func NewRedisChart() RedisChart {
	return RedisChart{
		BaseStatelessAppSpec: spec.BaseStatelessAppSpec{
			BaseServiceSpec: spec.BaseServiceSpec{ServiceName: redisName},
		},
	}
}

// ReadOnlyRootFilesystem overrides the cdk8s-plus default (true) to false.
// Redis writes to /tmp for lock files even when /data is a separate PVC mount.
func (c RedisChart) ReadOnlyRootFilesystem() *bool { return ptr.To(false) }

// Resources declares the CPU and memory budget for Redis containers.
// Redis is memory-heavy: memory requests should be close to limits to avoid OOM eviction.
// Convention: CPU:Memory ≈ 1 core : 4 Gi; requests < limits.
func (c RedisChart) Resources() *spec.ResourceRequirements {
	return &spec.ResourceRequirements{
		CPURequest:    "100m",
		CPULimit:      "200m",
		MemoryRequest: "128Mi",
		MemoryLimit:   "256Mi",
	}
}

func (c RedisChart) BuildFunc() func(cdk8s.App) {
	return func(app cdk8s.App) {
		chart := cdk8s.NewChart(app, jsii.String(c.Name()), &cdk8s.ChartProps{
			Namespace: jsii.String("default"),
		})
		constructs.NewStatelessAppConstruct(chart, c.Name(), &constructs.StatelessAppConstructProps{
			Name:      c.Name(),
			Namespace: "default",
			Image:     "redis:7-alpine",
			Replicas:  c.Replicas(),
			Port:                   6379,
			Resources:              c.Resources(),
			ReadOnlyRootFilesystem: c.ReadOnlyRootFilesystem(),
		})
	}
}
