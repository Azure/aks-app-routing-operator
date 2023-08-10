package infra

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"golang.org/x/sync/errgroup"
)

func (p Provisioned) Test(ctx context.Context) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("hello from " + p.Name)

	return nil
}

func Test(p []Provisioned) error {
	lgr := logger.FromContext(context.Background())
	lgr.Info("starting tests")
	defer lgr.Info("finished tests")

	var eg errgroup.Group

	for _, provisioned := range p {
		ctx := context.Background()
		lgr := logger.FromContext(ctx)
		ctx = logger.WithContext(ctx, lgr.With("infra", provisioned.Name))

		eg.Go(func() error {
			return provisioned.Test(ctx)
		})
	}

	return nil
}
