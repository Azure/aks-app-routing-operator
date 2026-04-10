package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// should match the names in root config/crd/bases directory
	externalDnsCrdFilename              = "approuting.kubernetes.azure.com_externaldnses.yaml"
	clusterExternalDnsCrdFilename       = "approuting.kubernetes.azure.com_clusterexternaldnses.yaml"
	nginxIngresscontrollerCrdFilename   = "approuting.kubernetes.azure.com_nginxingresscontrollers.yaml"
	defaultDomainCertificateCrdFilename = "approuting.kubernetes.azure.com_defaultdomaincertificates.yaml"
)

// readAllCRDs reads and parses all CRD files from the configured directory, returning them keyed by filename.
func readAllCRDs(cfg *config.Config, log logr.Logger) (map[string]*apiextensionsv1.CustomResourceDefinition, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	log = log.WithValues("crdPath", cfg.CrdPath)
	files, err := os.ReadDir(cfg.CrdPath)
	if err != nil {
		return nil, fmt.Errorf("reading crd directory %s: %w", cfg.CrdPath, err)
	}

	crds := make(map[string]*apiextensionsv1.CustomResourceDefinition)
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		path := filepath.Join(cfg.CrdPath, file.Name())
		log := log.WithValues("path", path)
		log.Info("reading crd file")
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading crd file %s: %w", path, err)
		}

		crd := &apiextensionsv1.CustomResourceDefinition{}
		log.Info("unmarshalling crd file")
		if err := yaml.UnmarshalStrict(content, crd); err != nil {
			return nil, fmt.Errorf("unmarshalling crd file %s: %w", path, err)
		}
		crds[file.Name()] = crd
	}
	return crds, nil
}

// loadCRDs loads the CRDs that should be active based on the current config into the cluster.
func loadCRDs(c client.Client, cfg *config.Config, log logr.Logger) error {
	crds, err := readAllCRDs(cfg, log)
	if err != nil {
		return err
	}

	for filename, crd := range crds {
		if !shouldLoadCRD(cfg, filename) {
			continue
		}
		log.Info("upserting crd", "name", crd.Name)
		if err := util.Upsert(context.Background(), c, crd); err != nil {
			return fmt.Errorf("upserting crd %s: %w", crd.Name, err)
		}
	}
	return nil
}

// removeDisabledCRDs removes CRDs from the cluster that are no longer needed based on the current config.
func removeDisabledCRDs(c client.Client, cfg *config.Config, log logr.Logger) error {
	crds, err := readAllCRDs(cfg, log)
	if err != nil {
		return err
	}

	for filename, crd := range crds {
		if !shouldRemoveCRD(cfg, filename) {
			continue
		}
		if err := removeCRD(c, crd, log); err != nil {
			return fmt.Errorf("removing crd %s: %w", crd.Name, err)
		}
	}
	return nil
}

// removeCRD deletes a CRD from the cluster if it exists.
// This is used to clean up CRDs that were previously installed but are no longer needed.
func removeCRD(c client.Client, crd *apiextensionsv1.CustomResourceDefinition, log logr.Logger) error {
	log = log.WithValues("name", crd.Name)
	log.Info("deleting crd")
	if err := c.Delete(context.Background(), crd); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("crd already deleted, nothing to do")
			return nil
		}
		return fmt.Errorf("deleting crd %s: %w", crd.Name, err)
	}

	log.Info("crd deleted successfully")
	return nil
}

// shouldRemoveCRD returns true if the CRD should be actively removed from the cluster.
// This is the inverse cleanup logic for CRDs that were previously installed but are now disabled.
func shouldRemoveCRD(cfg *config.Config, filename string) bool {
	switch filename {
	case nginxIngresscontrollerCrdFilename:
		return cfg.DisableIngressNginx
	default:
		return false
	}
}

func shouldLoadCRD(cfg *config.Config, filename string) bool {
	switch filename {
	case nginxIngresscontrollerCrdFilename:
		return !cfg.DisableIngressNginx

	case externalDnsCrdFilename:
		return cfg.EnabledWorkloadIdentity

	// ClusterExternalDNS CRD is also needed when default domain is enabled because
	// the default domain DNS reconciler creates a ClusterExternalDNS CR to manage DNS records
	case clusterExternalDnsCrdFilename:
		return cfg.EnabledWorkloadIdentity || cfg.EnableDefaultDomain

	case defaultDomainCertificateCrdFilename:
		return cfg.EnableDefaultDomain

	default:
		return false
	}
}
