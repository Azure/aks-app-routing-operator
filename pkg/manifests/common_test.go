package manifests

import (
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	namespaceTestCases = []struct {
		Name   string
		Config *config.Config
	}{
		{Name: "namespace", Config: &config.Config{
			NS: "test-namespace",
		}},
		{
			Name: "another-namespace",
			Config: &config.Config{
				NS: "another-test-namespace",
			},
		},
	}
)

func TestNamespaceResources(t *testing.T) {
	for _, tc := range namespaceTestCases {
		objs := namespace(tc.Config)
		fixture := path.Join("fixtures", "common", tc.Name) + ".json"
		AssertFixture(t, fixture, []client.Object{objs})
	}
}
