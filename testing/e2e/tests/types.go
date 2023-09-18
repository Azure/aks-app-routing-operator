package tests

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"k8s.io/client-go/rest"
)

type test interface {
	GetName() string
	Run(ctx context.Context, config *rest.Config) error
}

// T is an interface for a single test
type T interface {
	// GetOperatorConfigs returns a slice of OperatorConfig structs that should be used for this test.
	// All OperatorConfigs that are compatible should be returned.
	GetOperatorConfigs() []manifests.OperatorConfig
	test
}

// Ts is a slice of T
type Ts []T

type ordered []testsWithRunInfo

type testsWithRunInfo struct {
	tests                  []test
	config                 manifests.OperatorConfig
	operatorDeployStrategy operatorDeployStrategy
}

type testWithConfig struct {
	test
	config manifests.OperatorConfig
}

type operatorDeployStrategy int

const (
	// upgrade simulates the upgrade scenario for an existing user.
	upgrade operatorDeployStrategy = iota
	// cleanDeploy deletes the operator if it exists and then deploys it. This simulates a new user.
	cleanDeploy
)

func (o operatorDeployStrategy) string() string {
	switch o {
	case upgrade:
		return "upgrade"
	case cleanDeploy:
		return "cleanDeploy"
	default:
		panic("unknown operator deploy strategy")
	}
}
