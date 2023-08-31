package tests

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"golang.org/x/sync/errgroup"
)

func (tests Ts) Run(ctx context.Context, infra infra.Provisioned) error {
	var parallel []T
	var sequential []T

	lgr := logger.FromContext(ctx)

	// divide tests according to run strategy and filter out tests that should not run
	for _, test := range tests {
		if !test.ShouldRun(infra) {
			lgr.Info("skipping test", "test", test.GetName())
		}

		if test.GetRunStrategy() == Parallel {
			parallel = append(parallel, test)
		} else {
			sequential = append(sequential, test)
		}
	}

	runTestFn := func(t T) *logger.LoggedError {
		lgr := lgr.With("test", t.GetName())
		ctx := logger.WithContext(ctx, lgr)
		lgr.Info("starting to run test")

		if err := t.Run(ctx); err != nil {
			return logger.Error(lgr, err)
		}

		lgr.Info("finished running test")
		return nil
	}

	lgr.Info("running sequential tests")
	for _, test := range sequential {
		if err := runTestFn(test); err != nil {
			return err
		}
	}
	lgr.Info("finished running sequential tests")

	lgr.Info("running parallel tests")
	var eg errgroup.Group
	for _, p := range parallel {
		func(test T) {
			eg.Go(func() error {
				return runTestFn(test)
			})
		}(p)
	}

	if err := eg.Wait(); err != nil {
		return err
	}
	lgr.Info("finished running parallel tests")

	return nil
}
