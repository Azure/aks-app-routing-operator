// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller"
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().Unix())

	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if err := config.Flags.Validate(); err != nil {
		return err
	}

	if err := config.Flags.Validate2(); err != nil {
		return err
	}

	mgr, err := controller.NewManager(config.Flags)
	if err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}
