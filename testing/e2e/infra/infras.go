package infra

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/google/uuid"
)

var (
	rg       = "app-routing-e2e" + uuid.New().String()
	location = "South Central US"
)

var (
	// OsmInfraName is the name of the infrastructure that will be used to test an OSM enable cluster
	OsmInfraName = "osm cluster"
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
		Name:          OsmInfraName,
		ResourceGroup: rg,
		Location:      location,
		Suffix:        uuid.New().String(),
		McOpts: []clients.McOpt{
			func(mc *armcontainerservice.ManagedCluster) error {
				if mc.Properties.AddonProfiles == nil {
					mc.Properties.AddonProfiles = map[string]*armcontainerservice.ManagedClusterAddonProfile{}
				}

				mc.Properties.AddonProfiles["openServiceMesh"] = &armcontainerservice.ManagedClusterAddonProfile{
					Enabled: to.Ptr(true),
				}

				return nil
			},
		},
	},
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
