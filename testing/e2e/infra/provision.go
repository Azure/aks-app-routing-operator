package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"golang.org/x/sync/errgroup"
)

const (
	// lenZones is the number of zones to provision
	lenZones = 2
	// lenPrivateZones is the number of private zones to provision
	lenPrivateZones = 2
)

func (i *infra) Provision(ctx context.Context, tenantId, subscriptionId string) (Provisioned, *logger.LoggedError) {
	lgr := logger.FromContext(ctx)
	lgr.Info("provisioning infrastructure " + i.Name)
	defer lgr.Info("finished provisioning infrastructure " + i.Name)

	ret := Provisioned{
		Name:           i.Name,
		SubscriptionId: subscriptionId,
		TenantId:       tenantId,
	}

	var err error
	ret.ResourceGroup, err = clients.NewResourceGroup(ctx, subscriptionId, i.ResourceGroup, i.Location, clients.DeleteAfterOpt(2*time.Hour))
	if err != nil {
		return Provisioned{}, logger.Error(lgr, fmt.Errorf("creating resource group %s: %w", i.ResourceGroup, err))
	}

	// create resources
	var resEg errgroup.Group
	resEg.Go(func() error {
		ret.ContainerRegistry, err = clients.NewAcr(ctx, subscriptionId, i.ResourceGroup, "registry"+i.Suffix, i.Location)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating container registry: %w", err))
		}
		return nil
	})

	resEg.Go(func() error {
		ret.Cluster, err = clients.NewAks(ctx, subscriptionId, i.ResourceGroup, "cluster"+i.Suffix, i.Location, i.McOpts...)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating managed cluster: %w", err))
		}
		return nil
	})

	resEg.Go(func() error {
		ret.KeyVault, err = clients.NewAkv(ctx, tenantId, subscriptionId, i.ResourceGroup, "keyvault"+i.Suffix, i.Location)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating key vault: %w", err))
		}

		ret.Cert, err = ret.KeyVault.CreateCertificate(ctx, "cert"+i.Suffix, []string{"" + i.Suffix})
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating certificate: %w", err))
		}

		return nil
	})

	for idx := 0; idx < lenZones; idx++ {
		// need to perform loop variable capture on the index.
		// https://github.com/golang/go/wiki/LoopvarExperiment
		// there is a proposed change for 1.21 https://tip.golang.org/blog/go1.21rc
		// that will change the loop variable capture to be the standard way loops work.
		func(idx int) {
			resEg.Go(func() error {
				zone, err := clients.NewZone(ctx, subscriptionId, i.ResourceGroup, fmt.Sprintf("zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating zone: %w", err))
				}
				ret.Zones = append(ret.Zones, zone)
				return nil
			})
		}(idx)
	}
	for idx := 0; idx < lenPrivateZones; idx++ {
		func(idx int) {
			resEg.Go(func() error {
				privateZone, err := clients.NewPrivateZone(ctx, subscriptionId, i.ResourceGroup, fmt.Sprintf("private-zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating private zone: %w", err))
				}
				ret.PrivateZones = append(ret.PrivateZones, privateZone)
				return nil
			})
		}(idx)
	}

	if err := resEg.Wait(); err != nil {
		return Provisioned{}, logger.Error(lgr, err)
	}

	// connect permissions
	var permEg errgroup.Group

	for _, pz := range ret.PrivateZones {
		func(pz privateZone) {
			permEg.Go(func() error {
				cluster, err := ret.Cluster.GetCluster(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting cluster: %w", err))
				}

				dns, err := pz.GetDnsZone(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting dns: %w", err))
				}

				principalId := cluster.Identity.PrincipalID
				role := clients.PrivateDnsContributorRole
				if _, err := clients.NewRoleAssignment(ctx, subscriptionId, *dns.ID, *principalId, role); err != nil {
					return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
				}

				vnet, err := ret.Cluster.GetVnetId(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting vnet id: %w", err))
				}
				if err := pz.LinkVnet(ctx, fmt.Sprintf("link-%s-%s", pz.GetName(), i.Suffix), vnet); err != nil {
					return logger.Error(lgr, fmt.Errorf("linking vnet: %w", err))
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
					return logger.Error(lgr, fmt.Errorf("getting cluster: %w", err))
				}

				dns, err := z.GetDnsZone(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting dns: %w", err))
				}

				principalId := cluster.Identity.PrincipalID
				role := clients.DnsContributorRole
				if _, err := clients.NewRoleAssignment(ctx, subscriptionId, *dns.ID, *principalId, role); err != nil {
					return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
				}

				return nil
			})
		}(z)
	}

	permEg.Go(func() error {
		cluster, err := ret.Cluster.GetCluster(ctx)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("getting cluster: %w", err))
		}

		kubelet, ok := cluster.Properties.IdentityProfile["kubeletidentity"]
		if !ok {
			return logger.Error(lgr, fmt.Errorf("kubelet identity not found"))
		}
		principalId := kubelet.ObjectID

		role := clients.AcrPullRole
		scope := ret.ContainerRegistry.GetId()
		if _, err := clients.NewRoleAssignment(ctx, subscriptionId, scope, *principalId, role); err != nil {
			return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
		}

		return nil
	})

	permEg.Go(func() error {
		cluster, err := ret.Cluster.GetCluster(ctx)
		if err != nil {
			return fmt.Errorf("getting cluster: %w", err)
		}

		principalId := cluster.Identity.PrincipalID
		if err := ret.KeyVault.AddAccessPolicy(ctx, *principalId, armkeyvault.Permissions{
			Certificates: []*armkeyvault.CertificatePermissions{to.Ptr(armkeyvault.CertificatePermissionsGet)},
			Secrets:      []*armkeyvault.SecretPermissions{to.Ptr(armkeyvault.SecretPermissionsGet)},
		}); err != nil {
			return logger.Error(lgr, fmt.Errorf("adding access policy: %w", err))
		}

		return nil
	})

	if err := permEg.Wait(); err != nil {
		return Provisioned{}, logger.Error(lgr, err)
	}

	return ret, nil
}

func (is infras) Provision(tenantId, subscriptionId string) ([]Provisioned, error) {
	lgr := logger.FromContext(context.Background())
	lgr.Info("starting to provision all infrastructure")
	defer lgr.Info("finished provisioning all infrastructure")

	var eg errgroup.Group
	provisioned := make([]Provisioned, len(is))

	for idx, inf := range is {
		func(idx int, inf infra) {
			eg.Go(func() error {
				ctx := context.Background()
				lgr := logger.FromContext(ctx)
				ctx = logger.WithContext(ctx, lgr.With("infra", inf.Name))

				provisionedInfra, err := inf.Provision(ctx, tenantId, subscriptionId)
				if err != nil {
					return fmt.Errorf("provisioning infrastructure %s: %w", inf.Name, err)
				}

				provisioned[idx] = provisionedInfra
				return nil
			})
		}(idx, inf)
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return provisioned, nil
}
