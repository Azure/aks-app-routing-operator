package manifests

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/gator/reader"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/gator/test"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

const genFixturesEnv = "GENERATE_FIXTURES"

const constraintsPath = "./policy/manifests"

// initLoggerOnce ensures the logger is only initialized once to prevent race conditions
// when tests run in parallel
var initLoggerOnce sync.Once

func initLogger() {
	initLoggerOnce.Do(func() {
		log.SetLogger(zap.New(zap.UseDevMode(true)))
	})
}

var namespaceTestCases = []struct {
	Name          string
	Config        *config.Config
	NamespaceName string
}{
	{
		Name:          "namespace",
		NamespaceName: "test-namespace",
		Config:        &config.Config{},
	},
	{
		Name:          "another-namespace",
		NamespaceName: "another-test-namespace",
		Config:        &config.Config{},
	},
}

func TestNamespaceResources(t *testing.T) {
	t.Parallel()

	for _, tc := range namespaceTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			objs := Namespace(tc.Config, tc.NamespaceName)
			fixture := path.Join("fixtures", "common", tc.Name) + ".yaml"
			AssertFixture(t, fixture, []client.Object{objs})
		})
	}
}

func TestHasTopLevelLabels(t *testing.T) {
	t.Parallel()

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

func TestWithLivenessProbeMatchingReadinessNewFailureThresh(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		inputContainer *corev1.Container
		failureThresh  int32
		expected       corev1.Container
	}{
		{
			name: "empty readiness",
			inputContainer: &corev1.Container{
				Name: "name",
			},
			failureThresh: 2,
			expected: corev1.Container{
				Name: "name",
			},
		},
		{
			name: "new failure thresh",
			inputContainer: &corev1.Container{
				Name: "name",
				ReadinessProbe: &corev1.Probe{
					FailureThreshold: 2,
				},
			},
			failureThresh: 10,
			expected: corev1.Container{
				Name: "name",
				ReadinessProbe: &corev1.Probe{
					FailureThreshold: 2,
				},
				LivenessProbe: &corev1.Probe{
					FailureThreshold: 10,
				},
			},
		},
		{
			name: "new failure thresh with other fields",
			inputContainer: &corev1.Container{
				Name: "name",
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
						},
					},
					FailureThreshold:    2,
					SuccessThreshold:    1,
					TimeoutSeconds:      2,
					PeriodSeconds:       12,
					InitialDelaySeconds: 3,
				},
			},
			failureThresh: 200,
			expected: corev1.Container{
				Name: "name",
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
						},
					},
					FailureThreshold:    2,
					SuccessThreshold:    1,
					TimeoutSeconds:      2,
					PeriodSeconds:       12,
					InitialDelaySeconds: 3,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
						},
					},
					FailureThreshold:    200,
					SuccessThreshold:    1,
					TimeoutSeconds:      2,
					PeriodSeconds:       12,
					InitialDelaySeconds: 3,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := withLivenessProbeMatchingReadinessNewFailureThresh(c.inputContainer, c.failureThresh)
			require.Equal(t, c.expected, *got)
		})
	}
}

func TestGetOwnerRefs(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	actual := []byte{}
	for _, obj := range objs {
		marshalled, err := yaml.Marshal(obj)
		require.NoError(t, err)
		actual = append(actual, marshalled...)
		actual = append(actual, []byte("---\n")...)
	}

	if os.Getenv(genFixturesEnv) != "" {
		err := os.MkdirAll(filepath.Dir(fixturePath), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(fixturePath, actual, 0o644)
		require.NoError(t, err)
	}

	expected, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual), "expected and actual do not match for fixture %s", fixturePath)
}

type GatekeeperException struct {
	MessageSuffix string // use the suffix because expansion templates modify the prefix
	Constraint    string
}

func (g GatekeeperException) Ignores(r *test.GatorResult) bool {
	if !strings.HasSuffix(r.Msg, g.MessageSuffix) {
		return false
	}

	if r.Constraint == nil || r.Constraint.GetName() != g.Constraint {
		return false
	}

	return true
}

func GatekeeperTest(t *testing.T, manifestPath string, exceptions ...GatekeeperException) {
	// Initialize logger required by gatekeeper v3.20.1+
	// This prevents "eventuallyFulfillRoot" panic when test.Test() tries to log
	// Use sync.Once to prevent race conditions when tests run in parallel
	initLogger()

	// similar to https://github.com/open-policy-agent/gatekeeper/blob/master/cmd/gator/test/test.go
	unstructs, err := reader.ReadSources([]string{constraintsPath, manifestPath}, []string{}, "")
	require.NoError(t, err, "reading manifest", "path", manifestPath)
	require.True(t, len(unstructs) > 0, "no manifests found", "path", manifestPath)

	responses, err := test.Test(unstructs, test.Opts{})
	require.NoError(t, err, "auditing objects")

	for _, res := range responses.Results() {
		if res.EnforcementAction == "deny" {
			ignore := false
			for _, exception := range exceptions {
				if exception.Ignores(res) {
					ignore = true
					break
				}
			}

			if ignore {
				continue
			}

			require.Fail(t, res.Msg, "constraint", res.Constraint.GetName())
		}
	}
}
