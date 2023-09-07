package tests

import (
	"context"
	"fmt"
	"sort"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func init() {
	log.SetLogger(logr.New(log.NullLogSink{})) // without this controller-runtime panics. We use it solely for the client so we can ignore logs
}

func (t Ts) Run(ctx context.Context, infra infra.Provisioned) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("determining testing order")
	ordered := t.order(ctx)

	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("getting in-cluster config: %w", err)
	}

	runTestFn := func(t test, ctx context.Context) *logger.LoggedError {
		lgr := logger.FromContext(ctx).With("test", t.GetName())
		ctx = logger.WithContext(ctx, lgr)
		lgr.Info("starting to run test")

		if err := t.Run(ctx, config); err != nil {
			return logger.Error(lgr, err)
		}

		lgr.Info("finished running test")
		return nil
	}

	publicZones := make([]string, len(infra.Zones))
	for i, zone := range infra.Zones {
		publicZones[i] = zone.GetId()
	}
	privateZones := make([]string, len(infra.PrivateZones))
	for i, zone := range infra.PrivateZones {
		privateZones[i] = zone.GetId()
	}

	for i, runStrategy := range ordered {
		lgr.Info("run strategy testing order",
			"index", i,
			"operatorVersion", runStrategy.config.Version.String(),
			"operatorDeployStrategy", runStrategy.operatorDeployStrategy.string(),
			"privateZones", runStrategy.config.Zones.Private.String(),
			"publicZones", runStrategy.config.Zones.Public.String(),
			"disableOsm", runStrategy.config.DisableOsm,
		)
	}

	lgr.Info("starting to run tests")
	for _, runStrategy := range ordered {
		ctx := logger.WithContext(ctx, lgr.With(
			"operatorVersion", runStrategy.config.Version.String(),
			"operatorDeployStrategy", runStrategy.operatorDeployStrategy.string(),
			"privateZones", runStrategy.config.Zones.Private.String(),
			"publicZones", runStrategy.config.Zones.Public.String(),
			"disableOsm", runStrategy.config.DisableOsm,
		))
		if err := deployOperator(ctx, config, runStrategy.operatorDeployStrategy, infra.OperatorImage, publicZones, privateZones, &runStrategy.config); err != nil {
			return fmt.Errorf("deploying operator: %w", err)
		}

		var eg errgroup.Group
		for _, t := range runStrategy.tests {
			func(t test) {
				eg.Go(func() error {
					if err := runTestFn(t, ctx); err != nil {
						return fmt.Errorf("running test: %w", err)
					}

					return nil
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

// order builds the testing order for the given tests
func (t Ts) order(ctx context.Context) ordered {
	lgr := logger.FromContext(ctx)

	// group tests by operator version
	lgr.Info("grouping tests by operator version")
	operatorVersionSet := make(map[manifests.OperatorVersion][]testWithConfig)
	for _, test := range t {
		for _, config := range test.GetOperatorConfigs() {
			withConfig := testWithConfig{
				test:   test,
				config: config,
			}
			operatorVersionSet[config.Version] = append(operatorVersionSet[config.Version], withConfig)
		}
	}

	lgr.Info("operator version set", "operatorVersionSet", operatorVersionSet)

	// order operator versions in ascending order
	versions := keys(operatorVersionSet)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})
	lgr.Info("sorted operator versions", "versions", versions)

	if len(versions) == 0 { // would mean no tests were supplied
		return nil
	}
	if versions[len(versions)-1] != manifests.OperatorVersionLatest { // this should be impossible
		panic("operatorVersionLatest should always be the last version in the sorted versions")
	}

	// combine tests that use the same operator configuration and operator version, so they can run in parallel
	ret := make(ordered, 0, len(t))
	for _, version := range versions {
		operatorCfgSet := make(map[manifests.OperatorConfig][]testWithConfig)
		for _, test := range operatorVersionSet[version] {
			operatorCfgSet[test.config] = append(operatorCfgSet[test.config], test)
		}

		testsForVersion := make([]testsWithRunInfo, 0)
		for cfg := range operatorCfgSet {
			if cfg.Version != version {
				continue
			}

			var tests []test
			for _, test := range operatorCfgSet[cfg] {
				if test.config.Version != version {
					continue
				}

				tests = append(tests, test.test)
			}

			testsForVersion = append(ret, testsWithRunInfo{
				tests:                  tests,
				config:                 cfg,
				operatorDeployStrategy: upgrade,
			})
		}
		ret = append(ret, testsForVersion...)
		lgr.Info("tests for version", "version", version, "tests", testsForVersion)

		// operatorVersionLatest should always be the last version in the sorted versions
		if version == manifests.OperatorVersionLatest {
			// need to add cleanDeploy tests for the latest version (this is the version we are testing)
			new := make([]testsWithRunInfo, 0, len(testsForVersion))
			for _, tests := range testsForVersion {
				new = append(new, testsWithRunInfo{
					tests:                  tests.tests,
					config:                 tests.config,
					operatorDeployStrategy: cleanDeploy,
				})
			}
			ret = append(ret, new...)
		}

		lgr.Info("ret", "ret", ret)
	}

	return ret
}

func deployOperator(ctx context.Context, config *rest.Config, strategy operatorDeployStrategy, latestImage string, publicZones, privateZones []string, operatorCfg *manifests.OperatorConfig) error {
	lgr := logger.FromContext(ctx)

	c, err := client.New(config, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	cl, err := client.New(config, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	toDeploy := manifests.Operator(latestImage, publicZones, privateZones, operatorCfg)

	if strategy == cleanDeploy {
		lgr.Info("cleaning old operators")
		for _, res := range toDeploy {
			if res.GetObjectKind().GroupVersionKind().Kind == "Namespace" {
				continue // don't clean up namespace because we will get into race condition of namespace being in terminating state. It's fine to leave it, it's really deleting the other things that we care about.
			}

			copy := res.DeepCopyObject().(client.Object) // need copy so original object is not mutated
			if err := cl.Delete(ctx, copy); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting resource: %w", err)
			}
		}
	}

	lgr.Info("deploying operator")
	for _, res := range toDeploy {
		lgr.Info("deploying resource", "kind", res.GetObjectKind().GroupVersionKind().Kind, "name", res.GetName())
		err := c.Patch(ctx, res, client.Apply, client.ForceOwnership, client.FieldOwner("test-operator"))
		if apierrors.IsNotFound(err) {
			err = c.Create(ctx, res, client.FieldOwner("test-operator"))
		}
		if err != nil {
			return fmt.Errorf("creating or updating resource: %w", err)
		}
	}

	return nil
}

func keys[T comparable, V any](m map[T]V) []T {
	ret := make([]T, 0, len(m))
	for k := range m {
		ret = append(ret, k)
	}

	return ret
}
