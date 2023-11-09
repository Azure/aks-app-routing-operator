package manifests

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		objs := Namespace(tc.Config)
		fixture := path.Join("fixtures", "common", tc.Name) + ".json"
		AssertFixture(t, fixture, []client.Object{objs})
	}
}

func TestHasTopLevelLabels(t *testing.T) {
	cases := []struct {
		Labels    map[string]string
		ReqLabels []map[string]string
		Outcome   bool
	}{
		{Labels: map[string]string{}, Outcome: false},                                                               // Blank labels
		{Labels: map[string]string{"fake": "fake"}, Outcome: false},                                                 // Only fake labels
		{Labels: map[string]string{"app.kubernetes.io/managed-by": "false-operator-name"}, Outcome: false},          // Correct key, incorrect value
		{Labels: GetTopLevelLabels(), Outcome: true},                                                                // Correct labels
		{Labels: util.MergeMaps(GetTopLevelLabels(), map[string]string{"fakeLabel1": "fakeValue1"}), Outcome: true}, // Additional labels
	}

	for _, c := range cases {
		require.Equal(t, HasTopLevelLabels(c.Labels), c.Outcome)
	}
}

func TestGetOwnerRefs(t *testing.T) {
	cases := []struct {
		Name       string
		Owner      client.Object
		Controller bool
		Outcome    []metav1.OwnerReference
	}{
		{
			Name: "non-controller",
			Owner: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					UID:  "test-uid",
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Namespace",
				},
			},
			Controller: false,
			Outcome: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       "test-namespace",
					UID:        "test-uid",
					Controller: util.ToPtr(false),
				},
			},
		},
		{
			Name: "controller",
			Owner: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment",
					UID:  "test-uid2",
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Deployment",
				},
			},
			Controller: true,
			Outcome: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Deployment",
					Name:       "test-deployment",
					UID:        "test-uid2",
					Controller: util.ToPtr(true),
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			ret := GetOwnerRefs(c.Owner, c.Controller)

			require.Equal(t, len(c.Outcome), len(ret))
			for i, ref := range ret {
				require.Equal(t, ref.APIVersion, c.Outcome[i].APIVersion)
				require.Equal(t, ref.Kind, c.Outcome[i].Kind)
				require.Equal(t, ref.Name, c.Outcome[i].Name)
				require.Equal(t, ref.UID, c.Outcome[i].UID)
				require.Equal(t, ref.Controller, c.Outcome[i].Controller)
			}
		})
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
