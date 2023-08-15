package test

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"golang.org/x/sync/errgroup"
)

// tests is a slice of all tests to be run
var t tests = tests{}

func (t tests) Run(ctx context.Context, infra infra.Provisioned) error {
	parallel := tests{}
	sequential := tests{}

	lgr := logger.FromContext(ctx)

	// divide tests according to run strategy and filter out tests that should not run
	for _, test := range t {
		if !test.ShouldRun(infra) {
			lgr.Info("skipping test", "test", test.GetName())
		}

		if test.GetRunStrategy() == Parallel {
			parallel = append(parallel, test)
		} else {
			sequential = append(sequential, test)
		}
	}

	runTestFn := func(t Test) *logger.LoggedError {
		lgr := lgr.With("test", t.GetName())
		ctx := logger.WithContext(ctx, lgr)
		lgr.Info("starting to run test")
		defer lgr.Info("finished running test")

		if err := t.Run(ctx); err != nil {
			return logger.Error(lgr, err)
		}

		return nil
	}

	lgr.Info("running sequential tests")
	for _, test := range sequential {
		if err := runTestFn(test); err != nil {
			return err
		}
	}

	lgr.Info("running parallel tests")
	var eg errgroup.Group
	for _, p := range parallel {
		func(test Test) {
			eg.Go(func() error {
				return runTestFn(test)
			})
		}(p)
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}
