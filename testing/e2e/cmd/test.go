package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/spf13/cobra"
)

func init() {
	setupInfraFileFlag(testCmd)
	rootCmd.AddCommand(testCmd)
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Runs e2e tests against provisioned infrastructure",
	RunE: func(cmd *cobra.Command, args []string) error {
		file, err := os.Open(infraFile)
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer file.Close()

		bytes, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		var loaded []infra.LoadableProvisioned
		if err := json.Unmarshal(bytes, &loaded); err != nil {
			return fmt.Errorf("unmarshalling saved infrastructure: %w", err)
		}

		provisioned, err := infra.ToProvisioned(loaded)
		if err != nil {
			return fmt.Errorf("generating provisioned infrastructure: %w", err)
		}

		if err := infra.Test(provisioned); err != nil {
			return fmt.Errorf("test failed: %w", err)
		}

		return nil
	},
}
