package suites

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/tests"
	"k8s.io/client-go/rest"
)

// All returns all test in all suites
func All(infra infra.Provisioned) tests.Ts {
	t := []test{}
	t = append(t, basicSuite(infra)...)
	t = append(t, osmSuite(infra)...)
	t = append(t, promSuite(infra)...)
	t = append(t, nicTests(infra)...)
	t = append(t, externalDnsCrdTests(infra)...)
	t = append(t, clusterExternalDnsCrdTests(infra)...)
	t = append(t, defaultBackendTests(infra)...)
	t = append(t, workloadIdentityTests(infra)...)
	ret := make(tests.Ts, len(t))
	for i, t := range t {
		ret[i] = t
	}

	return ret
}

type test struct {
	name string
	cfgs operatorCfgs
	run  func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error
}

func (t test) GetName() string {
	return t.name
}

func (t test) GetOperatorConfigs() []manifests.OperatorConfig {
	return t.cfgs
}

func (t test) Run(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
	if t.run == nil {
		return fmt.Errorf("no run function provided for test %s", t.GetName())
	}

	return t.run(ctx, config, operator)
}

var alwaysRun = func(infra infra.Provisioned) bool {
	return true
}
