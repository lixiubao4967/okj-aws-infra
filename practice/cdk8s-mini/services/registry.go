// Package services is the single source of truth for all synthesized services.
// To add a new service: create a file in the appropriate subdirectory,
// then register it in All() below. cmd/main.go never needs to change.
package services

import (
	"cdk8s-mini/services/web"

	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

// ServiceChart is the contract every service must satisfy.
type ServiceChart interface {
	// Name returns the service identifier used as the output subdirectory name
	// (e.g., "nginx" → mini-charts/nginx/nginx.yaml).
	Name() string
	// BuildFunc returns the function that populates the cdk8s App with this service's charts.
	BuildFunc() func(cdk8s.App)
}

// All returns all services to synthesize.
// cmd/main.go iterates this list — add new services here, nowhere else.
func All() []ServiceChart {
	return []ServiceChart{
		web.NewNginxChart(),
	}
}
