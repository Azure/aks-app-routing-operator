// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"flag"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller"
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		lgr := controller.GetLogger()
		lgr.Error(err, "failed to run manager")
		os.Exit(1)
	}
}

func run() error {
	if err := config.Flags.Validate(); err != nil {
		return err
	}

	mgr, err := controller.NewManager(config.Flags)
	if err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}
