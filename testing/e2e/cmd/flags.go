package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

const (
	subscriptionIdFlag = "subscription"
	tenantIdFlag       = "tenant"
	infraNamesFlag     = "names"
)

var (
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

var (
	infraNames []string
)

func setupInfraNamesFlag(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&infraNames, infraNamesFlag, []string{}, "infrastructure names to provision, if empty will provision all")
}
