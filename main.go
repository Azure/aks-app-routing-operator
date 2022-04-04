// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/ingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
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

	mgr, err := newManager(config.Flags)
	if err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

func newManager(conf *config.Config) (ctrl.Manager, error) {
	m, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		return nil, err
	}
	ctrl.SetLogger(klogr.New())
	secv1.AddToScheme(m.GetScheme())

	if err = ingress.NewIngressControllerReconciler(m, manifests.IngressControllerResources(conf)); err != nil {
		return nil, err
	}
	if err = keyvault.NewIngressSecretProviderClassReconciler(m, conf); err != nil {
		return nil, err
	}
	if err = keyvault.NewIngressTLSReconciler(m); err != nil {
		return nil, err
	}
	if err = keyvault.NewPlaceholderPodController(m, conf); err != nil {
		return nil, err
	}

	return m, nil
}
