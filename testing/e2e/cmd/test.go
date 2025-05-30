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
		provisionedInfra := provisioned[0]

		tests := suites.All(provisionedInfra)
		if err := tests.Run(context.Background(), provisionedInfra); err != nil {
			return logger.Error(lgr, fmt.Errorf("test failed: %w", err))
		}

		if err := provisionedInfra.Cleanup(ctx); err != nil {
			lgr.Error(fmt.Sprintf("cleaning up provisioned infrastructure: %s", err.Error()))
			// we purposefully don't return an error here, not worth marking the test as failed if cleanup fails.
			// garbage collection in the subscription will take care of the resources
		}

		return nil
	},
}
