package cmd

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/spf13/cobra"
)

func init() {
	setupInfraNameFlag(testCmd)
	rootCmd.AddCommand(testCmd)
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Runs e2e tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		lgr := logger.FromContext(context.Background())
		lgr.Info("Hello World from " + infraName)

		return nil
	},
}
