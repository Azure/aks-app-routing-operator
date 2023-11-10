package controller

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/webhook"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func OperatorInit(conf *config.Config, rc *rest.Config) error {
	lgr := getLogger().WithName("init")
	lgr.Info("setting up so operator can run")

	cl, err := client.New(rc, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	wh, err := webhook.New(conf)
	if err != nil {
		return fmt.Errorf("creating webhook configuration: %w", err)
	}

	if err := wh.EnsureCertificates(context.Background(), lgr, cl); err != nil {
		return fmt.Errorf("ensuring webhook certificates: %w", err)
	}

	lgr.Info("operator setup complete")
	return nil
}
