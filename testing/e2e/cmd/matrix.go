package cmd

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/github"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/spf13/cobra"
)

func init() {
	setupInfraNamesFlag(matrixCmd)
	rootCmd.AddCommand(matrixCmd)
}

var matrixCmd = &cobra.Command{
	Use:   "matrix",
	Short: "Prints the GitHub workflow matrix for the tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		infras := infra.Infras
		if len(infraNames) > 0 {
			infras = infras.FilterNames(infraNames)
		}

		infraNamers := []github.Namer{}
		for _, infra := range infras {
			infraNamers = append(infraNamers, namer{infra.Name})
		}

		matrix, err := github.NameMatrix(infraNamers)
		if err != nil {
			return fmt.Errorf("creating matrix: %w", err)
		}

		github.SetOutput("matrix", matrix)

		return nil
	},
}

type namer struct {
	name string
}

func (n namer) Name() string {
	return n.name
}
