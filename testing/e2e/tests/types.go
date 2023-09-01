package tests

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

type test interface {
	GetName() string
	Run(ctx context.Context) error
}

// T is an interface for a single test
type T interface {
	// GetOperatorConfigs returns a slice of OperatorConfig structs that should be used for this test
	GetOperatorConfigs() []manifests.OperatorConfig
	ShouldRun(infra infra.Provisioned) bool
	test
}

// Ts is a slice of T
type Ts []T

type ordered []testsWithConfig

type testsWithConfig struct {
	tests  []test
	config manifests.OperatorConfig
}

type testWithConfig struct {
	test
	config manifests.OperatorConfig
}
