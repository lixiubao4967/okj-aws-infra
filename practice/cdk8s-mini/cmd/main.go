// Package main is the entry point for cdk8s chart synthesis.
// This file never changes — add new services in services/registry.go instead.
package main

import (
	"cdk8s-mini/services"
	"os"
	"path/filepath"

	"github.com/aws/jsii-runtime-go"
	"github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

func main() {
	// Allow overriding the output directory via environment variable.
	baseDir := "mini-charts"
	if v := os.Getenv("CDK8S_OUTDIR"); v != "" {
		baseDir = v
	}

	// Each service gets its own App and output subdirectory:
	//   mini-charts/<service-name>/<service-name>.yaml
	makeApp := func(name string) cdk8s.App {
		return cdk8s.NewApp(&cdk8s.AppProps{
			Outdir:              jsii.String(filepath.Join(baseDir, name)),
			OutputFileExtension: jsii.String(".yaml"),
			YamlOutputType:      cdk8s.YamlOutputType_FILE_PER_CHART,
		})
	}

	for _, svc := range services.All() {
		app := makeApp(svc.Name())
		svc.BuildFunc()(app)
		app.Synth()
	}
}
