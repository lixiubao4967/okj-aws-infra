// Package web contains chart specifications for web-tier services.
package web

import (
	"cdk8s-mini/internal/constructs"
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
			Port:      80, // jsii.Number() 是给 jsii 运行时的包装，普通 Go int 不需要
		})
	}
}
