package controller

import (
	"context"
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
	validCrdPath        = "../../config/crd/bases/"
	validCrdName        = "nginxingresscontrollers.approuting.kubernetes.azure.com"
	validCrdPathWithDir = "../../config/crd/"

	nonCrdManifestsPath = "../manifests/fixtures/nginx"
	nonExistentFilePath = "./this/does/not/exist"
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

}
