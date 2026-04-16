// Package web contains chart specifications for web-tier services.
package web

import (
	"cdk8s-mini/internal/constructs"
	"cdk8s-mini/internal/ptr"
	"cdk8s-mini/internal/spec"

	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

const nginxName = "nginx"

// NginxChart declares the requirements for the nginx deployment.
// It embeds BaseStatelessAppSpec for default implementations of Name(), Replicas(), Resources().
type NginxChart struct {
	spec.BaseStatelessAppSpec
}

// NewNginxChart constructs an NginxChart with the service name set.
func NewNginxChart() NginxChart {
	return NginxChart{
		BaseStatelessAppSpec: spec.BaseStatelessAppSpec{
			BaseServiceSpec: spec.BaseServiceSpec{ServiceName: nginxName},
		},
	}
}

// ReadOnlyRootFilesystem overrides the cdk8s-plus default (true) to false.
// nginx writes to /var/cache/nginx and /var/run — needs a writable root FS.
func (c NginxChart) ReadOnlyRootFilesystem() *bool { return ptr.To(false) }

// Resources declares the CPU and memory budget for nginx containers.
// Overrides BaseServiceSpec.Resources() which returns nil (= no limits).
// Convention: CPU:Memory ≈ 1 core : 4 Gi; requests < limits.
func (c NginxChart) Resources() *spec.ResourceRequirements {
	// TODO: 返回 nginx 合适的资源限制
	// nginx 是轻量静态服务器，不需要太多资源
	// 参考格式：CPURequest "100m", CPULimit "200m", MemoryRequest "64Mi", MemoryLimit "128Mi"
	return &spec.ResourceRequirements{
		CPURequest:    "100m",
		CPULimit:      "200m",
		MemoryRequest: "64Mi",
		MemoryLimit:   "128Mi",
	}
}

func (c NginxChart) BuildFunc() func(cdk8s.App) {
	return func(app cdk8s.App) {
		chart := cdk8s.NewChart(app, jsii.String(c.Name()), &cdk8s.ChartProps{
			Namespace: jsii.String("default"),
		})
		constructs.NewStatelessAppConstruct(chart, c.Name(), &constructs.StatelessAppConstructProps{
			Name:      c.Name(),
			Namespace: "default",
			Image:     "nginx:1.27-alpine",
			Replicas:  c.Replicas(),
			Resources:              c.Resources(),
			ReadOnlyRootFilesystem: c.ReadOnlyRootFilesystem(),
			Port:      80,
		})
	}
}
