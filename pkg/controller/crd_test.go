package controller

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	nginxCrdName              = "nginxingresscontrollers.approuting.kubernetes.azure.com"
	clusterExternalDnsCrdName = "clusterexternaldnses.approuting.kubernetes.azure.com"
	externalDnsCrdName        = "externaldnses.approuting.kubernetes.azure.com"
	managedCertificateCrdName = "managedcertificates.approuting.kubernetes.azure.com"

	validCrdPath        = "../../config/crd/bases/"
	validCrdName        = nginxCrdName
	validCrdPathWithDir = "../../config/crd/"

	nonCrdManifestsPath = "../manifests/fixtures/nginx/default_version"
	nonExistentFilePath = "./this/does/not/exist"
)

var (
	nginxCrds              = []string{nginxCrdName}
	externalDnsCrds        = []string{externalDnsCrdName, clusterExternalDnsCrdName}
	managedCertificateCrds = []string{managedCertificateCrdName}
)

var (
	gatewayEnabledManagedCertificateDisabled  = &config.Config{EnableGateway: true, CrdPath: validCrdPath}
	gatewayDisabledManagedCertificateDisabled = &config.Config{EnableGateway: false, CrdPath: validCrdPath}
	gatewayEnabledManagedCertificateEnabled   = &config.Config{EnableGateway: true, EnableManagedCertificates: true, CrdPath: validCrdPath}
	gatewayDisabledManagedCertificateEnabled  = &config.Config{EnableGateway: false, EnableManagedCertificates: true, CrdPath: validCrdPath}
)

func TestLoadCRDs(t *testing.T) {
	t.Run("valid crds", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, loadCRDs(cl, &config.Config{CrdPath: validCrdPath}, logr.Discard()), "expected no error loading valid crds")

		crd := &apiextensionsv1.CustomResourceDefinition{}
		crd.Name = validCrdName
		require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(crd), crd, nil), "getting loaded valid crd")
	})

	t.Run("valid crds with directory", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		require.NoError(t, loadCRDs(cl, &config.Config{CrdPath: validCrdPath}, logr.Discard()), "expected no error loading valid crds")
	})

	t.Run("invalid crds", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		err := loadCRDs(cl, &config.Config{CrdPath: nonCrdManifestsPath}, logr.Discard())
		require.Error(t, err, "expected error loading invalid crds")
		require.True(t, strings.Contains(err.Error(), "unmarshalling crd file"), "expected error to be about umarshalling crd")
	})

	t.Run("non-existent crd path", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		err := loadCRDs(cl, &config.Config{CrdPath: nonExistentFilePath}, logr.Discard())
		require.Error(t, err, "expected error loading non-existent crd path")
		require.True(t, strings.Contains(err.Error(), "reading crd directory"), "expected error to be about reading crd directory")
	})

	t.Run("nil config", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		err := loadCRDs(cl, nil, logr.Discard())
		require.Error(t, err, "expected error loading nil config")
		require.True(t, strings.Contains(err.Error(), "config cannot be nil"), "expected error to be about nil config")
	})

	// prove it doesn't load unwanted crds
	cases := []struct {
		name             string
		cfg              *config.Config
		expectedCRDNames []string
	}{
		{name: "gateway enabled, managed certificate disabled", cfg: gatewayEnabledManagedCertificateDisabled, expectedCRDNames: slices.Concat(nginxCrds, externalDnsCrds)},
		{name: "gateway disabled, managed certificate disabled", cfg: gatewayDisabledManagedCertificateDisabled, expectedCRDNames: nginxCrds},
		{name: "gateway enabled, managed certificate enabled", cfg: gatewayEnabledManagedCertificateEnabled, expectedCRDNames: slices.Concat(nginxCrds, externalDnsCrds, managedCertificateCrds)},
		{name: "gateway disabled, managed certificate enabled", cfg: gatewayDisabledManagedCertificateEnabled, expectedCRDNames: slices.Concat(nginxCrds, managedCertificateCrds)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			require.NoError(t, loadCRDs(cl, tc.cfg, logr.Discard()), "expected no error loading crds")

			crds := &apiextensionsv1.CustomResourceDefinitionList{}
			require.NoError(t, cl.List(context.Background(), crds), "expected no error listing crds")

			seen := map[string]struct{}{}
			for _, crd := range crds.Items {
				seen[crd.Name] = struct{}{}
			}

			require.True(t, len(seen) == len(tc.expectedCRDNames), "expected correct number of crds")
			for _, expected := range tc.expectedCRDNames {
				_, ok := seen[expected]
				require.True(t, ok, fmt.Sprintf("expected crd %s to be loaded", expected))
			}
		})
	}
}

func TestShouldLoadCRD(t *testing.T) {
	// prove that crd filenames are correct
	crdFiles, err := os.ReadDir(validCrdPath)
	require.NoError(t, err, "expected no error reading crd directory")
	seen := map[string]bool{
		externalDnsCrdFilename:        false,
		clusterExternalDnsCrdFilename: false,
	}
	for _, file := range crdFiles {
		seen[file.Name()] = true
	}
	for filename, expected := range seen {
		require.True(t, expected, fmt.Sprintf("expected crd with filename %s to exist in %s", filename, validCrdPath))
	}

	cases := []struct {
		name     string
		cfg      *config.Config
		filename string
		expected bool
	}{
		{name: "external dns crd with gateway enabled", cfg: gatewayEnabledManagedCertificateDisabled, filename: externalDnsCrdFilename, expected: true},
		{name: "external dns crd with gateway disabled", cfg: gatewayDisabledManagedCertificateDisabled, filename: externalDnsCrdFilename, expected: false},
		{name: "cluster external dns crd with gateway enabled", cfg: gatewayEnabledManagedCertificateDisabled, filename: clusterExternalDnsCrdFilename, expected: true},
		{name: "cluster external dns crd with gateway disabled", cfg: gatewayDisabledManagedCertificateDisabled, filename: clusterExternalDnsCrdFilename, expected: false},
		{name: "other crd with gateway enabled", cfg: gatewayEnabledManagedCertificateEnabled, filename: "other.crd.yaml", expected: true},
		{name: "other crd with gateway disabled", cfg: gatewayDisabledManagedCertificateEnabled, filename: "other.crd.yaml", expected: true},
		{name: "managed certificate crd with managed certificates enabled", cfg: gatewayEnabledManagedCertificateEnabled, filename: managedCertificateCrdFilename, expected: true},
		{name: "managed certificate crd with managed certificates disabled", cfg: gatewayDisabledManagedCertificateDisabled, filename: managedCertificateCrdFilename, expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, shouldLoadCRD(tc.cfg, tc.filename), "expected correct crd loading behavior")
		})
	}
}
