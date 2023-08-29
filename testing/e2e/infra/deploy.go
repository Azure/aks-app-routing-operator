package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"golang.org/x/sync/errgroup"
)

func (p Provisioned) Deploy(ctx context.Context) error {
	lgr := logger.FromContext(ctx).With("infra", p.Name)
	lgr.Info("deploying tests")
	defer lgr.Info("finished deploying tests")

	loadable, err := ToLoadable([]Provisioned{p})
	if err != nil {
		return logger.Error(lgr, fmt.Errorf("failed to convert provisioned to loadable: %w", err))
	}

	jsonLoadable, err := json.Marshal(loadable)
	if err != nil {
		return logger.Error(lgr, fmt.Errorf("failed to marshal loadable: %w", err))
	}

	objs := manifests.E2e(p.E2eImage, string(jsonLoadable))
	if err := p.Cluster.Clean(ctx, objs); err != nil {
		return logger.Error(lgr, err)
	}

	if err := p.Cluster.Deploy(ctx, objs); err != nil {
		return logger.Error(lgr, err)
	}

	return nil
}

func Deploy(p []Provisioned) error {
	lgr := logger.FromContext(context.Background())
	lgr.Info("deploying tests")
	defer lgr.Info("finished deploying tests")

	var eg errgroup.Group

	for _, provisioned := range p {
		ctx := context.Background()
		lgr := logger.FromContext(ctx)
		ctx = logger.WithContext(ctx, lgr.With("infra", provisioned.Name))

		eg.Go(func() error {
			return provisioned.Deploy(ctx)
		})
	}

	if err := eg.Wait(); err != nil {
		return logger.Error(lgr, err)
	}

	return nil
}
