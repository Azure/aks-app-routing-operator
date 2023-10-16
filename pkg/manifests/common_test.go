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

const genFixturesEnv = "GENERATE_FIXTURES"

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

func TestHasTopLevelLabels(t *testing.T) {
	cases := []struct {
		Labels  map[string]string
		Outcome bool
	}{
		{Labels: map[string]string{}, Outcome: false},
		{Labels: map[string]string{"fake": "fake"}, Outcome: false},
		{Labels: map[string]string{"app.kubernetes.io/managed-by": "false-operator-name"}, Outcome: false},
		{Labels: map[string]string{"fakeLabel1": "fakeValue1", "fakeLabel2": "fakeValue2", "fakeLabel3": "fakeValue3", "app.kubernetes.io/managed-by": "aks-app-routing-operator"}, Outcome: true},
	}

	for _, c := range cases {
		require.Equal(t, HasTopLevelLabels(c.Labels), c.Outcome)
	}
}

// AssertFixture checks the fixture path and compares it to the provided objects, failing if they are not equal
func AssertFixture(t *testing.T, fixturePath string, objs []client.Object) {
	t.Logf("Testing fixture %s", fixturePath)
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
