package suites

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/tests"
)

// All returns all test in all suites
func All() tests.Ts {
	t := []test{}
	t = append(t, exampleSuite()...)

	ret := make(tests.Ts, len(t))
	for i, t := range t {
		ret[i] = t
	}

	return ret
}

type test struct {
	name      string
	strategy  tests.RunStrategy
	run       func(ctx context.Context) error
	shouldRun func(infra infra.Provisioned) bool
}

func (t test) GetName() string {
	return t.name
}

func (t test) GetRunStrategy() tests.RunStrategy {
	return t.strategy
}

func (t test) Run(ctx context.Context) error {
	if t.run == nil {
		return fmt.Errorf("no run function provided for test %s", t.GetName())
	}

	return t.run(ctx)
}

func (t test) ShouldRun(infra infra.Provisioned) bool {
	if t.shouldRun == nil {
		return true
	}

	return t.shouldRun(infra)
}

var alwaysRun = func(infra infra.Provisioned) bool {
	return true
}
