package tests

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
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

	// order operator versions in ascending order
	versions := keys(operatorVersionSet)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})

	if len(versions) == 0 { // would mean no tests were supplied
		return nil
	}
	if versions[len(versions)-1] != manifests.OperatorVersionLatest { // this should be impossible
		panic("operatorVersionLatest should always be the last version in the sorted versions")
	}

	// combine tests that use the same operator configuration and operator version, so they can run in parallel
	lgr.Info("grouping tests by operator configuration")
	ret := make(ordered, 0)
	for _, version := range versions {
		// group tests by operator configuration
		operatorCfgSet := make(map[manifests.OperatorConfig][]testWithConfig)
		for _, test := range operatorVersionSet[version] {
			operatorCfgSet[test.config] = append(operatorCfgSet[test.config], test)
		}

		testsForVersion := make([]testsWithRunInfo, 0)
		for cfg, tests := range operatorCfgSet {
			var casted []test
			for _, test := range tests {
				casted = append(casted, test.test)
			}

			testsForVersion = append(testsForVersion, testsWithRunInfo{
				tests:                  casted,
				config:                 cfg,
				operatorDeployStrategy: upgrade,
			})
		}
		ret = append(ret, testsForVersion...)

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
			// don't cleanup the namespace, we will reuse it. it's just wasted time
			if res.GetObjectKind().GroupVersionKind().Kind == "Namespace" {
				continue
			}

			if err := cl.Delete(ctx, res); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting resource: %w", err)
			}
		}

		lgr.Info("cleaning old testing namespaces")
		var list corev1.NamespaceList
		if err := cl.List(ctx, &list, client.MatchingLabels{manifests.ManagedByKey: manifests.ManagedByVal}); err != nil {
			return fmt.Errorf("listing testing namespaces: %w", err)
		}

		for _, ns := range list.Items {
			if err := cl.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting testing namespace: %w", err)
			}
		}

		// wait for namespaces to be fully deleted, we will redeploy same namespace for clean deploy scenarios
		for _, ns := range list.Items {
			if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
				var copy corev1.Namespace
				if err := cl.Get(ctx, client.ObjectKey{Name: ns.Name}, &copy); err != nil {
					if apierrors.IsNotFound(err) {
						return true, nil
					}

					return false, fmt.Errorf("getting namespace: %w", err)
				}

				return false, nil
			}); err != nil {
				return fmt.Errorf("waiting for namespace to be deleted: %w", err)
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

		// if res is deployment, wait for it to be ready
		if res.GetObjectKind().GroupVersionKind().Kind == "Deployment" {
			if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
				var copy appsv1.Deployment
				if err := c.Get(ctx, client.ObjectKeyFromObject(res), &copy); err != nil {
					return false, fmt.Errorf("getting deployment: %w", err)
				}

				// check rollout status of deployment
				if copy.Status.UpdatedReplicas != *copy.Spec.Replicas {
					return false, nil
				}

				return true, nil
			}); err != nil {
				return fmt.Errorf("waiting for deployment to be ready: %w", err)
			}
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
