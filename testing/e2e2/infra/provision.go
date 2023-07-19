package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/config"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"golang.org/x/sync/errgroup"
)

func (i *Infra) Provision(ctx context.Context) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("provisioning infrastructure " + i.Name)
	defer lgr.Info("finished provisioning infrastructure " + i.Name)

	_, err := clients.NewResourceGroup(ctx, config.Flags.SubscriptionId, i.ResourceGroup, i.Location, clients.DeleteAfterOpt(2*time.Hour))
	if err != nil {
		lgr.Error("failed to create resource group", "error", err)
		return fmt.Errorf("creating resource group %s: %w", i.ResourceGroup, err)
	}

	_, err = clients.NewAcr(ctx, config.Flags.SubscriptionId, i.ResourceGroup, "registry"+i.Suffix, i.Location)
	if err != nil {
		lgr.Error("failed to create container registry", "error", err)
		return fmt.Errorf("creating container registry: %w", err)
	}

	_, err = clients.NewAks(ctx, config.Flags.SubscriptionId, i.ResourceGroup, "cluster"+i.Suffix, i.Location, i.McOpts...)
	if err != nil {
		lgr.Error("failed to create managed cluster", "error", err)
		return fmt.Errorf("creating managed cluster: %w", err)
	}

	return nil
}

func (is Infras) Provision() error {
	lgr := logger.FromContext(context.Background())
	lgr.Info("starting to provision all infrastructure")
	defer lgr.Info("finished provisioning all infrastructure")

	var eg errgroup.Group
	for _, i := range is {
		func(i Infra) {
			eg.Go(func() error {
				ctx := context.Background()
				lgr := logger.FromContext(ctx)
				ctx = logger.WithContext(ctx, lgr.With("infra", i.Name))
				lgr.With("testing arg", "val").Info("test")
				return i.Provision(ctx)
			})
		}(i)
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}
