package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/suites"
	"github.com/spf13/cobra"
)

func init() {
	setupInfraFileFlag(testCmd)
	rootCmd.AddCommand(testCmd)
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Runs e2e tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		lgr := logger.FromContext(ctx)

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

		if len(provisioned) != 1 {
			return fmt.Errorf("expected 1 provisioned infrastructure, got %d", len(provisioned))
		}

		tests := suites.All(provisioned[0])
		if err := tests.Run(context.Background(), provisioned[0]); err != nil {
			return logger.Error(lgr, fmt.Errorf("test failed: %w", err))
		}

		return nil
	},
}
