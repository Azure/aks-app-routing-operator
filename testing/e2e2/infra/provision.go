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

const (
	// lenZones is the number of zones to provision
	lenZones = 2
	// lenPrivateZones is the number of private zones to provision
	lenPrivateZones = 2
)

func (i *Infra) Provision(ctx context.Context) (ProvisionedInfra, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("provisioning infrastructure " + i.Name)
	defer lgr.Info("finished provisioning infrastructure " + i.Name)

	ret := ProvisionedInfra{
		Name: i.Name,
	}

	var err error
	ret.ResourceGroup, err = clients.NewResourceGroup(ctx, config.Flags.SubscriptionId, i.ResourceGroup, i.Location, clients.DeleteAfterOpt(2*time.Hour))
	if err != nil {
		lgr.Error("failed to create resource group", "error", err)
		return ProvisionedInfra{}, fmt.Errorf("creating resource group %s: %w", i.ResourceGroup, err)
	}

	// create resources
	var resEg errgroup.Group
	resEg.Go(func() error {
		ret.ContainerRegistry, err = clients.NewAcr(ctx, config.Flags.SubscriptionId, i.ResourceGroup, "registry"+i.Suffix, i.Location)
		if err != nil {
			return fmt.Errorf("creating container registry: %w", err)
		}
		return nil
	})

	resEg.Go(func() error {
		ret.Cluster, err = clients.NewAks(ctx, config.Flags.SubscriptionId, i.ResourceGroup, "cluster"+i.Suffix, i.Location, i.McOpts...)
		if err != nil {
			return fmt.Errorf("creating managed cluster: %w", err)
		}
		return nil
	})

	resEg.Go(func() error {
		ret.KeyVault, err = clients.NewAkv(ctx, config.Flags.SubscriptionId, i.ResourceGroup, "keyvault"+i.Suffix, i.Location)
		if err != nil {
			return fmt.Errorf("creating key vault: %w", err)
		}

		ret.Cert, err = ret.KeyVault.CreateCertificate(ctx, "cert"+i.Suffix, []string{"" + i.Suffix})
		return nil
	})

	for idx := 0; idx < lenZones; idx++ {
		func(idx int) {
			resEg.Go(func() error {
				zone, err := clients.NewZone(ctx, config.Flags.SubscriptionId, i.ResourceGroup, fmt.Sprintf("zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return fmt.Errorf("creating zone: %w", err)
				}
				ret.Zones = append(ret.Zones, zone)
				return nil
			})
		}(idx)
	}
	for idx := 0; idx < lenPrivateZones; idx++ {
		func(idx int) {
			resEg.Go(func() error {
				privateZone, err := clients.NewPrivateZone(ctx, config.Flags.SubscriptionId, i.ResourceGroup, fmt.Sprintf("private-zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return fmt.Errorf("creating private zone: %w", err)
				}
				ret.PrivateZones = append(ret.PrivateZones, privateZone)
				return nil
			})
		}(idx)
	}

	if err := resEg.Wait(); err != nil {
		return ProvisionedInfra{}, err
	}

	// connect permissions
	var permEg errgroup.Group

	for _, pz := range ret.PrivateZones {
		func(pz privateZone) {
			permEg.Go(func() error {
				cluster, err := ret.Cluster.GetCluster(ctx)
				if err != nil {
					return fmt.Errorf("getting cluster: %w", err)
				}

				dns, err := pz.GetDns(ctx)
				if err != nil {
					return fmt.Errorf("getting dns: %w", err)
				}

				principalId := cluster.Identity.PrincipalID
				if _, err := clients.NewRoleAssignment(ctx, config.Flags.SubscriptionId, *dns.ID, "Private DNS Zone Contributor", *principalId); err != nil {
					return fmt.Errorf("creating Private DNS Zone Contributor role assignment: %w", err)
				}

				vnet, err := ret.Cluster.GetVnetId(ctx)
				if err != nil {
					return fmt.Errorf("getting vnet id: %w", err)
				}
				if err := pz.LinkVnet(ctx, fmt.Sprintf("link-%s-%s", pz.GetName(), i.Suffix), vnet); err != nil {
					return fmt.Errorf("linking vnet: %w", err)
				}

				return nil
			})
		}(pz)
	}

	for _, z := range ret.Zones {
		func(z zone) {
			permEg.Go(func() error {
				cluster, err := ret.Cluster.GetCluster(ctx)
				if err != nil {
					return fmt.Errorf("getting cluster: %w", err)
				}

				dns, err := z.GetDns(ctx)
				if err != nil {
					return fmt.Errorf("getting dns: %w", err)
				}

				principalId := cluster.Identity.PrincipalID
				if _, err := clients.NewRoleAssignment(ctx, config.Flags.SubscriptionId, *dns.ID, "DNS Zone Contributor", *principalId); err != nil {
					return fmt.Errorf("creating DNS Zone Contributor role assignment: %w", err)
				}

				return nil
			})
		}(z)
	}

	permEg.Go(func() error {
		cluster, err := ret.Cluster.GetCluster(ctx)
		if err != nil {
			return fmt.Errorf("getting cluster: %w", err)
		}

		kubelet, ok := cluster.Properties.IdentityProfile["kubeletidentity"]
		if !ok {
			return fmt.Errorf("kubelet identity not found")
		}
		principalId := kubelet.ObjectID

		if _, err := clients.NewRoleAssignment(ctx, config.Flags.SubscriptionId, ret.ContainerRegistry.GetId(), "AcrPull", *principalId); err != nil {
			return fmt.Errorf("creating AcrPull role assignment: %w", err)
		}

		return nil
	})

	permEg.Go(func() error {
		// todo: which identity to give vault permissions to? should be the same as dns but they are different?
		cluster, err := ret.Cluster.GetCluster(ctx)
		if err != nil {
			return fmt.Errorf("getting cluster: %w", err)
		}

		if _, err := clients.NewRoleAssignment(ctx, config.Flags.SubscriptionId, ret.KeyVault.GetId(), "Key Vault Secrets User", *cluster.Identity.PrincipalID); err != nil {
			return fmt.Errorf("creating Key Vault Secrets User role assignment: %w", err)
		}

		return nil
	})

	if err := permEg.Wait(); err != nil {
		return ProvisionedInfra{}, err
	}

	return ret, nil
}

func (is Infras) Provision() ([]ProvisionedInfra, error) {
	lgr := logger.FromContext(context.Background())
	lgr.Info("starting to provision all infrastructure")
	defer lgr.Info("finished provisioning all infrastructure")

	var eg errgroup.Group
	provisioned := make([]ProvisionedInfra, len(is))

	for idx, infra := range is {
		func(idx int, infra Infra) {
			eg.Go(func() error {
				ctx := context.Background()
				lgr := logger.FromContext(ctx)
				ctx = logger.WithContext(ctx, lgr.With("infra", infra.Name))

				provisionedInfra, err := infra.Provision(ctx)
				if err != nil {
					return fmt.Errorf("provisioning infrastructure %s: %w", infra.Name, err)
				}

				provisioned[idx] = provisionedInfra
				return nil
			})
		}(idx, infra)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return provisioned, nil
}
