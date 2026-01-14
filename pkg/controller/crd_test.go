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
	defaultDomainCertCrdName  = "defaultdomaincertificates.approuting.kubernetes.azure.com"

	validCrdPath        = "../../config/crd/bases/"
	validCrdName        = nginxCrdName
	validCrdPathWithDir = "../../config/crd/"

	nonCrdManifestsPath = "./testutils/testcrds/"
	nonExistentFilePath = "./this/does/not/exist"
)

var (
	nginxCrds                 = []string{nginxCrdName}
	externalDnsCrds           = []string{externalDnsCrdName, clusterExternalDnsCrdName}
	managedCertificateCrds    = []string{managedCertificateCrdName}
	defaultDomainCertificates = []string{defaultDomainCertCrdName}
)

var (
	workloadIdentityEnabled                = &config.Config{EnabledWorkloadIdentity: true, CrdPath: validCrdPath}
	workloadIdentityAndGatewayEnabled      = &config.Config{EnabledWorkloadIdentity: true, EnableGateway: true, CrdPath: validCrdPath}
	workloadIdentityDisabled               = &config.Config{EnabledWorkloadIdentity: false, CrdPath: validCrdPath}
	workloadIdentityDisabledGatewayEnabled = &config.Config{EnabledWorkloadIdentity: false, EnableGateway: true, CrdPath: validCrdPath}
	defaultDomainEnabled                   = &config.Config{EnableDefaultDomain: true, CrdPath: validCrdPath}
	defaultDomainDisabled                  = &config.Config{EnableDefaultDomain: false, CrdPath: validCrdPath}
	allFeaturesEnabled                     = &config.Config{EnabledWorkloadIdentity: true, EnableGateway: true, EnableDefaultDomain: true, CrdPath: validCrdPath}
	allFeaturesDisabled                    = &config.Config{EnabledWorkloadIdentity: false, EnableDefaultDomain: false, CrdPath: validCrdPath}
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
		require.True(t, strings.Contains(err.Error(), "unmarshalling crd file"), "expected error to be about unmarshalling crd, instead was "+err.Error())
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
		{name: "workload identity enabled", cfg: workloadIdentityEnabled, expectedCRDNames: slices.Concat(nginxCrds, []string{clusterExternalDnsCrdName, externalDnsCrdName})},
		{name: "workload identity and gateway enabled", cfg: workloadIdentityAndGatewayEnabled, expectedCRDNames: slices.Concat(nginxCrds, []string{clusterExternalDnsCrdName, externalDnsCrdName})},
		{name: "workload identity disabled", cfg: workloadIdentityDisabled, expectedCRDNames: nginxCrds},
		{name: "workload identity disabled with gateway enabled", cfg: workloadIdentityDisabledGatewayEnabled, expectedCRDNames: nginxCrds},
		{name: "default domain enabled", cfg: defaultDomainEnabled, expectedCRDNames: slices.Concat(nginxCrds, defaultDomainCertificates)},
		{name: "default domain disabled", cfg: defaultDomainDisabled, expectedCRDNames: nginxCrds},
		{name: "all features enabled", cfg: allFeaturesEnabled, expectedCRDNames: slices.Concat(nginxCrds, []string{clusterExternalDnsCrdName, externalDnsCrdName}, defaultDomainCertificates)},
		{name: "all features disabled", cfg: allFeaturesDisabled, expectedCRDNames: nginxCrds},
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

			require.True(t, len(seen) == len(tc.expectedCRDNames), fmt.Sprintf("expected correct number of crds, seen: %v, expected: %v", seen, tc.expectedCRDNames))
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
		externalDnsCrdFilename:              false,
		clusterExternalDnsCrdFilename:       false,
		defaultDomainCertificateCrdFilename: false,
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
		{name: "external dns crd with workload identity enabled", cfg: workloadIdentityEnabled, filename: externalDnsCrdFilename, expected: true},
		{name: "external dns crd with workload identity and gateway enabled", cfg: workloadIdentityAndGatewayEnabled, filename: externalDnsCrdFilename, expected: true},
		{name: "external dns crd with workload identity disabled", cfg: workloadIdentityDisabled, filename: externalDnsCrdFilename, expected: false},
		{name: "external dns crd with workload identity disabled but gateway enabled", cfg: workloadIdentityDisabledGatewayEnabled, filename: externalDnsCrdFilename, expected: false},
		{name: "cluster external dns crd with workload identity enabled", cfg: workloadIdentityEnabled, filename: clusterExternalDnsCrdFilename, expected: true},
		{name: "cluster external dns crd with workload identity disabled", cfg: workloadIdentityDisabled, filename: clusterExternalDnsCrdFilename, expected: false},
		{name: "nginx ingress controller crd with workload identity enabled", cfg: workloadIdentityEnabled, filename: nginxIngresscontrollerCrdFilename, expected: true},
		{name: "nginx ingress controller crd with workload identity disabled", cfg: workloadIdentityDisabled, filename: nginxIngresscontrollerCrdFilename, expected: true},
		{name: "default domain certificate crd with default domain enabled", cfg: defaultDomainEnabled, filename: defaultDomainCertificateCrdFilename, expected: true},
		{name: "default domain certificate crd with default domain disabled", cfg: defaultDomainDisabled, filename: defaultDomainCertificateCrdFilename, expected: false},
		{name: "default domain certificate crd with all features enabled", cfg: allFeaturesEnabled, filename: defaultDomainCertificateCrdFilename, expected: true},
		{name: "default domain certificate crd with all features disabled", cfg: allFeaturesDisabled, filename: defaultDomainCertificateCrdFilename, expected: false},
		{name: "other crd with workload identity enabled", cfg: workloadIdentityEnabled, filename: "other.crd.yaml", expected: false},
		{name: "other crd with workload identity disabled", cfg: workloadIdentityDisabled, filename: "other.crd.yaml", expected: false},
		{name: "other crd with default domain enabled", cfg: defaultDomainEnabled, filename: "other.crd.yaml", expected: false},
		{name: "other crd with default domain disabled", cfg: defaultDomainDisabled, filename: "other.crd.yaml", expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, shouldLoadCRD(tc.cfg, tc.filename), "expected correct crd loading behavior")
		})
	}
}
