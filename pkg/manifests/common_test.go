package manifests

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// AssertFixture checks the fixture path and compares it to the provided objects, failing if they are not equal
func AssertFixture(t *testing.T, fixturePath string, objs []client.Object) {
	actual, err := json.MarshalIndent(&objs, "  ", "  ")
	require.NoError(t, err)

	if os.Getenv(genFixturesEnv) != "" {
		err = os.WriteFile(fixturePath, actual, 0644)
		require.NoError(t, err)
	}

	expected, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(actual))
}
