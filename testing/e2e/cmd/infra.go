package cmd

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/spf13/cobra"
)

func init() {
	setupSubTenantFlags(infraCmd)
	setupInfraNamesFlag(infraCmd)
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

		if _, err := infras.Provision(tenantId, subscriptionId); err != nil {
			return fmt.Errorf("provisioning infrastructure: %w", err)
		}

		return nil
	},
}
