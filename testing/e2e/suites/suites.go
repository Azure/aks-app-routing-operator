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

	ret := make(tests.Ts, len(t))
	for i, t := range t {
		ret[i] = t
	}

	return ret
}

type operatorCfgs []manifests.OperatorConfig

func (o operatorCfgs) WithAllOsm() operatorCfgs {
	ret := make([]manifests.OperatorConfig, 0, len(o)*2)
	for _, cfg := range o {
		copy := cfg
		copy2 := cfg
		copy.DisableOsm = false
		copy2.DisableOsm = true
		ret = append(ret, copy, copy2)
	}
	return ret
}

func (o operatorCfgs) withVersions(versions ...manifests.OperatorVersion) operatorCfgs {
	ret := make([]manifests.OperatorConfig, 0, len(o)*len(versions))
	for _, cfg := range o {
		for _, v := range versions {
			copy := cfg
			copy.Version = v
			ret = append(ret, copy)
		}
	}
	return ret
}

func (o operatorCfgs) withZones(zones ...manifests.DnsZones) operatorCfgs {
	ret := make([]manifests.OperatorConfig, 0, len(o)*len(zones))
	for _, cfg := range o {
		for _, z := range zones {
			copy := cfg
			copy.Zones = z
			ret = append(ret, copy)
		}
	}
	return ret
}

func (o operatorCfgs) withPublicZones(counts ...manifests.DnsZoneCount) operatorCfgs {
	ret := make([]manifests.OperatorConfig, 0, len(o)*len(counts))
	for _, cfg := range o {
		for _, c := range counts {
			copy := cfg
			copy.Zones.Public = c
			ret = append(ret, copy)
		}
	}
	return ret
}

func (o operatorCfgs) withPrivateZones(counts ...manifests.DnsZoneCount) operatorCfgs {
	ret := make([]manifests.OperatorConfig, 0, len(o)*len(counts))
	for _, cfg := range o {
		for _, c := range counts {
			copy := cfg
			copy.Zones.Private = c
			ret = append(ret, copy)
		}
	}
	return ret
}

type test struct {
	name string
	cfgs operatorCfgs
	run  func(ctx context.Context, config *rest.Config) error
}

func (t test) GetName() string {
	return t.name
}

func (t test) GetOperatorConfigs() []manifests.OperatorConfig {
	return t.cfgs
}

func (t test) Run(ctx context.Context, config *rest.Config) error {
	if t.run == nil {
		return fmt.Errorf("no run function provided for test %s", t.GetName())
	}

	return t.run(ctx, config)
}

var alwaysRun = func(infra infra.Provisioned) bool {
	return true
}
