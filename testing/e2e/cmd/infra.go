package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/spf13/cobra"
)

func init() {
	setupSubTenantFlags(infraCmd)
	setupInfraNamesFlag(infraCmd)
	setupInfraFileFlag(infraCmd)
	rootCmd.AddCommand(infraCmd)
}

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Sets up infrastructure for e2e tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		infras := infra.Infras
		if len(infraNames) > 0 {
			infras = infras.FilterNames(infraNames)
		}

		if len(infras) == 0 {
			return fmt.Errorf("no infrastructure configurations found")
		}

		provisioned, err := infras.Provision(tenantId, subscriptionId)
		if err != nil {
			return fmt.Errorf("provisioning infrastructure: %w", err)
		}

		loadable, err := infra.ToLoadable(provisioned)
		if err != nil {
			return fmt.Errorf("generating loadable infrastructure: %w", err)
		}

		file, err := os.Create(infraFile) // create truncates a file that exists
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer file.Close()

		bytes, err := json.Marshal(loadable)
		if err != nil {
			return fmt.Errorf("marshalling infrastructure: %w", err)
		}

		if _, err := file.Write(bytes); err != nil {
			return fmt.Errorf("writing infrastructure config: %w", err)
		}

		return nil
	},
}
