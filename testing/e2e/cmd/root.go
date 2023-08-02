package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

const (
	subscriptionIdFlag = "subscription"
	tenantIdFlag       = "tenant"
)

var (
	// global flags
	subscriptionId string
	tenantId       string
)

func setupSubTenantFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&subscriptionId, subscriptionIdFlag, "", "subscription")
	cmd.MarkFlagRequired(subscriptionIdFlag)
	cmd.Flags().StringVar(&tenantId, tenantIdFlag, "", "tenant")
	cmd.MarkFlagRequired(tenantIdFlag)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if subscriptionId == "" {
			return errors.New("subscription is required")
		}
		if tenantId == "" {
			return errors.New("tenant is required")
		}

		return nil
	}
}

var rootCmd = &cobra.Command{
	Use:   "e2e",
	Short: "e2e tests for the AKS App Routing Operator",
}

func Execute() error {
	return rootCmd.Execute()
}
