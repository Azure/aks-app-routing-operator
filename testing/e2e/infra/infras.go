package infra

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/google/uuid"
)

// Infras is a list of infrastructure configurations the e2e tests will run against
var Infras = infras{
	{
		Name:          "basic-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
	},
	{
		Name:          "private-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.PrivateClusterOpt},
	},
	{
		Name:          "osm-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.OsmClusterOpt, clients.VmCountOpt(8)}, // requires more VMs than other infras
	},
	{
		Name:                "gateway-full-mesh-cluster",
		ResourceGroup:       uniqueResourceGroup(),
		Location:            getLocation(),
		Suffix:              uuid.New().String()[:16],
		McOpts:              []clients.McOpt{clients.IstioServiceMeshOpt, clients.ManagedGatewayOpt},
		FederatedNamespaces: GenerateGatewayFederatedNamespaces(),
	},
	{
		Name:                "gateway-approuting-istio-cluster",
		ResourceGroup:       uniqueResourceGroup(),
		Location:            getLocation(),
		Suffix:              uuid.New().String()[:16],
		McOpts:              []clients.McOpt{clients.ManagedGatewayOpt, clients.AppRoutingIstioOpt},
		FederatedNamespaces: GenerateGatewayFederatedNamespaces(),
		PostCreate:          clients.EnableAppRoutingIstio,
	},

	// Dalec variants — identical AKS config, but the operator-config builder
	// detects DalecClusterOpt and sets dalecNginx=true / latest-only.
	{
		Name:          "basic-dalec-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.DalecClusterOpt},
	},
	{
		Name:          "private-dalec-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.PrivateClusterOpt, clients.DalecClusterOpt},
	},
	{
		Name:          "osm-dalec-cluster",
		ResourceGroup: uniqueResourceGroup(),
		Location:      getLocation(),
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.OsmClusterOpt, clients.VmCountOpt(8), clients.DalecClusterOpt},
	},
	{
		Name:                "gateway-full-mesh-dalec-cluster",
		ResourceGroup:       uniqueResourceGroup(),
		Location:            getLocation(),
		Suffix:              uuid.New().String()[:16],
		McOpts:              []clients.McOpt{clients.IstioServiceMeshOpt, clients.ManagedGatewayOpt, clients.DalecClusterOpt},
		FederatedNamespaces: GenerateGatewayFederatedNamespaces(),
	},
	{
		Name:                "gateway-approuting-istio-dalec-cluster",
		ResourceGroup:       uniqueResourceGroup(),
		Location:            getLocation(),
		Suffix:              uuid.New().String()[:16],
		McOpts:              []clients.McOpt{clients.ManagedGatewayOpt, clients.AppRoutingIstioOpt, clients.DalecClusterOpt},
		FederatedNamespaces: GenerateGatewayFederatedNamespaces(),
		PostCreate:          clients.EnableAppRoutingIstio,
	},

	// TODO: add back when service principal cluster is supported
	//{
	//	Name:                    "service principal cluster",
	//	ResourceGroup:           rg,
	//	Location:                location,
	//	Suffix:                  uuid.New().String(),
	//	McOpts:                  []clients.McOpt{},
	//	ServicePrincipalOptions: &clients.ServicePrincipalOptions{},
	//},
}

func (i infras) FilterNames(names []string) infras {
	ret := infras{}
	for _, infra := range i {
		for _, name := range names {
			if infra.Name == name {
				ret = append(ret, infra)
				break
			}
		}
	}

	return ret
}

func uniqueResourceGroup() string {
	return "app-routing-e2e" + uuid.New().String()
}
