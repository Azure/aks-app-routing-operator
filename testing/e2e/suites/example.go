package suites

import (
	"context"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/tests"
)

// TODO: this is just an example of a test suite for now. actual tests will be added here in a similar style in future PRs

func exampleSuite() []test {
	return []test{
		{
			name:     "example one",
			strategy: tests.Parallel,
			run: func(ctx context.Context) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("running example one")
				time.Sleep(10 * time.Second)
				lgr.Info("finished running example one")
				return nil
			},
			shouldRun: alwaysRun,
		},
		{
			name:     "example two",
			strategy: tests.Parallel,
			run: func(ctx context.Context) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("running example two")
				time.Sleep(10 * time.Second)
				lgr.Info("finished running example two")
				return nil
			},
			shouldRun: alwaysRun,
		},
	}
}
