package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "e2e",
	Short: "e2e tests for the AKS App Routing Operator",
}

func Execute() error {
	return rootCmd.Execute()
}
