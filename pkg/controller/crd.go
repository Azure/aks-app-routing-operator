package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// loadCRDs loads the CRDs from the specified path into the cluster
func loadCRDs(c client.Client, cfg *config.Config, log logr.Logger) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	log = log.WithValues("crdPath", cfg.CrdPath)
	log.Info("reading crd directory")
	files, err := os.ReadDir(cfg.CrdPath)
	if err != nil {
		return fmt.Errorf("reading crd directory %s: %w", cfg.CrdPath, err)
	}

	log.Info("applying crds")
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		path := filepath.Join(cfg.CrdPath, file.Name())
		log := log.WithValues("path", path)
		log.Info("reading crd file")
		var content []byte
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading crd file %s: %w", path, err)
		}

		log.Info("unmarshalling crd file")
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(content, crd); err != nil {
			return fmt.Errorf("unmarshalling crd file %s: %w", path, err)
		}

		log.Info("upserting crd")
		if err := util.Upsert(context.Background(), c, crd); err != nil {
			return fmt.Errorf("upserting crd %s: %w", crd.Name, err)
		}
	}

	log.Info("crds loaded")
	return nil
}
