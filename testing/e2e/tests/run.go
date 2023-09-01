package tests

import (
	"context"
	"sort"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"golang.org/x/sync/errgroup"
)

func (tests Ts) Run(ctx context.Context, infra infra.Provisioned) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("determining testing order")
	ordered := tests.order(ctx, infra)

	runTestFn := func(t test) *logger.LoggedError {
		lgr := lgr.With("test", t.GetName())
		ctx := logger.WithContext(ctx, lgr)
		lgr.Info("starting to run test")

		if err := t.Run(ctx); err != nil {
			return logger.Error(lgr, err)
		}

		lgr.Info("finished running test")
		return nil
	}

	lgr.Info("starting to run tests")
	for _, testsWithConfig := range ordered {
		cfg := testsWithConfig.config
		// TODO: need to provision deployment

		var eg errgroup.Group
		for _, t := range testsWithConfig.tests {
			func(t test) {
				eg.Go(func() error {
					return runTestFn(t)
				})
			}(t)
		}

		if err := eg.Wait(); err != nil {
			return err
		}
	}

	lgr.Info("successfully finished running tests")
	return nil
}

func (t Ts) order(ctx context.Context, infra infra.Provisioned) ordered {
	lgr := logger.FromContext(ctx)
	operatorVersionSet := make(map[manifests.OperatorVersion][]testWithConfig)

	for _, test := range t {
		if !test.ShouldRun(infra) {
			lgr.Info("skipping test", "test", test.GetName())
			continue
		}

		for _, config := range test.GetOperatorConfigs() {
			withConfig := testWithConfig{
				test:   test,
				config: config,
			}
			operatorVersionSet[config.Version] = append(operatorVersionSet[config.Version], withConfig)
		}
	}

	versions := keys(operatorVersionSet)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	ret := make(ordered, 0, len(t))
	for _, version := range versions {
		operatorCfgSet := make(map[manifests.OperatorConfig][]testWithConfig)
		for _, test := range operatorVersionSet[version] {
			operatorCfgSet[test.config] = append(operatorCfgSet[test.config], test)
		}

		for cfg := range operatorCfgSet {
			var tests []test
			for _, test := range operatorCfgSet[cfg] {
				tests = append(tests, test.test)
			}

			ret = append(ret, testsWithConfig{
				tests:  tests,
				config: cfg,
			})
		}
	}

	return ret
}

func keys[T comparable, V any](m map[T]V) []T {
	ret := make([]T, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}

	return ret
}
