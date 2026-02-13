package infra

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/utils"
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
		FederatedNamespaces: utils.GenerateGatewayFederatedNamespaces(),
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
