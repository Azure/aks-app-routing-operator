package cmd

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/spf13/cobra"
)

var (
	infraNames []string
)

func init() {
	setupSubTenantFlags(infraCmd)
	infraCmd.Flags().StringArrayVar(&infraNames, "names", []string{}, "infrastructure names to provision, if empty will provision all")
	rootCmd.AddCommand(infraCmd)
}

var infraCmd = &cobra.Command{
	Use:   "infra",
	Short: "Sets up infrastructure for e2e tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		infras := infra.Infras
		fmt.Println(infraNames)
		if len(infraNames) > 0 {
			infras = infras.FilterNames(infraNames)
		}

		if _, err := infras.Provision(tenantId, subscriptionId); err != nil {
			return fmt.Errorf("provisioning infrastructure: %w", err)
		}

		return nil
	},
}
