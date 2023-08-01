package main

import (
	"flag"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/config"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/google/uuid"
)

var (
	rg       = "app-routing-e2e" + uuid.New().String()
	location = "South Central US"
)

var infras = infra.Infras{
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
}

func main() {
	flag.Parse()
	if err := config.Flags.Validate(); err != nil {
		panic(err)
	}

	if _, err := infras.Provision(); err != nil {
		panic(fmt.Errorf("provisioning infrastructure: %w", err))
	}

}
