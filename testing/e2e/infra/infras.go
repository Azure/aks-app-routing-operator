package infra

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/google/uuid"
)

var (
	rg       = "app-routing-e2e" + uuid.New().String()
	location = "South Central US"
)

// Infras is a list of infrastructure configurations the e2e tests will run against
var Infras = infras{
	{
		Name:          "basic cluster",
		ResourceGroup: rg,
		Location:      location,
		Suffix:        uuid.New().String(),
	},
	{
		Name:          "private cluster",
		ResourceGroup: rg,
		Location:      location,
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.PrivateClusterOpt},
	},
	{
		Name:          "osm cluster",
		ResourceGroup: rg,
		Location:      location,
		Suffix:        uuid.New().String(),
		McOpts:        []clients.McOpt{clients.OsmClusterOpt},
	},
	// TODO: add back when service principal cluster is supported
	//{
	//	Name:                    "service principal cluster",
	//	ResourceGroup:           rg,
	//	Location:                location,
	//	Suffix:                  uuid.New().String(),
	//	McOpts:                  []clients.McOpt{clients.ServicePrincipalClusterOpt},
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
