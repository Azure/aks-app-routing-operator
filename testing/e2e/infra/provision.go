package infra

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
)

const (
	// lenZones is the number of zones to provision
	lenZones = 2
	// lenPrivateZones is the number of private zones to provision
	lenPrivateZones = 2
)

func (i *infra) Provision(ctx context.Context, tenantId, subscriptionId, applicationObjectId string) (Provisioned, *logger.LoggedError) {
	lgr := logger.FromContext(ctx).With("infra", i.Name)
	lgr.Info("provisioning infrastructure")
	defer lgr.Info("finished provisioning infrastructure")

	ret := Provisioned{
		Name:           i.Name,
		SubscriptionId: subscriptionId,
		TenantId:       tenantId,
	}

	if i.AuthType == AuthTypeServicePrincipal {
		if applicationObjectId == "" {
			return ret, logger.Error(lgr, fmt.Errorf("application object id must be provided when provisioning infrastructure with service principal options"))
		}
		credName := i.Name + "-cred"
		spOpt, err := clients.GetServicePrincipalOptions(ctx, applicationObjectId, credName)
		if err != nil {
			return ret, logger.Error(lgr, fmt.Errorf("getting app with credential: %w", err))
		}
		i.ServicePrincipal = spOpt
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

		resEg.Go(func() error {
			e2eRepoAndTag := "e2e:" + i.Suffix
			if err := ret.ContainerRegistry.BuildAndPush(ctx, e2eRepoAndTag, "../../", "../../Dockerfile.e2e"); err != nil {
				return logger.Error(lgr, fmt.Errorf("building and pushing e2e image: %w", err))
			}
			ret.E2eImage = ret.ContainerRegistry.GetName() + ".azurecr.io/" + e2eRepoAndTag
			return nil
		})

		resEg.Go(func() error {
			operatorRepoAndTag := "operator:" + i.Suffix
			if err := ret.ContainerRegistry.BuildAndPush(ctx, operatorRepoAndTag, "../../", "../../Dockerfile"); err != nil {
				return logger.Error(lgr, fmt.Errorf("building and pushing operator image: %w", err))
			}
			ret.OperatorImage = ret.ContainerRegistry.GetName() + ".azurecr.io/" + operatorRepoAndTag

			return nil
		})

		return nil
	})

	resEg.Go(func() error {
		ret.Cluster, err = clients.NewAks(ctx, subscriptionId, i.ResourceGroup, "cluster"+i.Suffix, i.Location, i.ServicePrincipal, i.McOpts...)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating managed cluster: %w", err))
		}
		return nil
	})

	kvDone := make(chan struct{})
	resEg.Go(func() error {
		defer close(kvDone)

		ret.KeyVault, err = clients.NewAkv(ctx, tenantId, subscriptionId, i.ResourceGroup, "keyvault"+i.Suffix, i.Location)
		if err != nil {
			return logger.Error(lgr, fmt.Errorf("creating key vault: %w", err))
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
				z, err := clients.NewZone(ctx, subscriptionId, i.ResourceGroup, fmt.Sprintf("Zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating Zone: %w", err))
				}

				<-kvDone

				cert, err := ret.KeyVault.CreateCertificate(ctx, fmt.Sprintf("Zone-%d", idx), z.GetName(), []string{z.GetName()})
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating certificate: %w", err))
				}

				ret.Zones = append(ret.Zones, WithCert[Zone]{
					Zone: z,
					Cert: cert,
				})
				return nil
			})
		}(idx)
	}
	for idx := 0; idx < lenPrivateZones; idx++ {
		func(idx int) {
			resEg.Go(func() error {
				pz, err := clients.NewPrivateZone(ctx, subscriptionId, i.ResourceGroup, fmt.Sprintf("private-Zone-%d-%s", idx, i.Suffix))
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating private Zone: %w", err))
				}

				<-kvDone

				cert, err := ret.KeyVault.CreateCertificate(ctx, fmt.Sprintf("privatezone-%d", idx), pz.GetName(), []string{pz.GetName()})
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("creating certificate: %w", err))
				}

				ret.PrivateZones = append(ret.PrivateZones, WithCert[PrivateZone]{
					Zone: pz,
					Cert: cert,
				})
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
		func(pz WithCert[PrivateZone]) {
			permEg.Go(func() error {
				dns, err := pz.Zone.GetDnsZone(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting dns: %w", err))
				}

				principalId := ret.Cluster.GetPrincipalId()
				role := clients.PrivateDnsContributorRole
				if _, err := clients.NewRoleAssignment(ctx, subscriptionId, *dns.ID, principalId, role); err != nil {
					return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
				}

				vnet, err := ret.Cluster.GetVnetId(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting vnet id: %w", err))
				}
				if err := pz.Zone.LinkVnet(ctx, fmt.Sprintf("link-%s-%s", pz.Zone.GetName(), i.Suffix), vnet); err != nil {
					return logger.Error(lgr, fmt.Errorf("linking vnet: %w", err))
				}

				return nil
			})
		}(pz)
	}

	for _, z := range ret.Zones {
		func(z WithCert[Zone]) {
			permEg.Go(func() error {
				dns, err := z.Zone.GetDnsZone(ctx)
				if err != nil {
					return logger.Error(lgr, fmt.Errorf("getting dns: %w", err))
				}

				principalId := ret.Cluster.GetPrincipalId()
				role := clients.DnsContributorRole
				if _, err := clients.NewRoleAssignment(ctx, subscriptionId, *dns.ID, principalId, role); err != nil {
					return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
				}

				return nil
			})
		}(z)
	}

	permEg.Go(func() error {
		principalId := ret.Cluster.GetPrincipalId()
		role := clients.AcrPullRole
		scope := ret.ContainerRegistry.GetId()
		if _, err := clients.NewRoleAssignment(ctx, subscriptionId, scope, principalId, role); err != nil {
			return logger.Error(lgr, fmt.Errorf("creating %s role assignment: %w", role.Name, err))
		}

		return nil
	})

	permEg.Go(func() error {
		principalId := ret.Cluster.GetPrincipalId()
		if err := ret.KeyVault.AddAccessPolicy(ctx, principalId, armkeyvault.Permissions{
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

func (is infras) Provision(tenantId, subscriptionId, applicationObjectId string) ([]Provisioned, error) {
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

				provisionedInfra, err := inf.Provision(ctx, tenantId, subscriptionId, applicationObjectId)
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
