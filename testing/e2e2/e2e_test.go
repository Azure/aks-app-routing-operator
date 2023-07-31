package e2e2

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/config"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/infra"
	"github.com/google/uuid"
)

var infras = infra.Infras{
	{
		Name:          "basic cluster",
		ResourceGroup: "app-routing-e2e" + uuid.New().String(),
		Location:      "South Central US",
		Suffix:        uuid.New().String(),
	},
}

func TestMain(m *testing.M) {
	flag.Parse()
	if err := config.Flags.Validate(); err != nil {
		panic(err)
	}

	if _, err := infras.Provision(); err != nil {
		panic(fmt.Errorf("provisioning infrastructure: %w", err))
	}

	os.Exit(m.Run())
}
