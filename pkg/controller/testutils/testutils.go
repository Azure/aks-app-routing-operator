package testutils

import (
	"path/filepath"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	promDTO "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func GetErrMetricCount(t *testing.T, controllerName controllername.ControllerNamer) float64 {
	errMetric, err := metrics.AppRoutingReconcileErrors.GetMetricWithLabelValues(controllerName.MetricsName())
	require.NoError(t, err)

	metricProto := &promDTO.Metric{}

	err = errMetric.Write(metricProto)
	require.NoError(t, err)

	beforeCount := metricProto.GetCounter().GetValue()
	return beforeCount
}

func GetReconcileMetricCount(t *testing.T, controllerName controllername.ControllerNamer, label string) float64 {
	errMetric, err := metrics.AppRoutingReconcileTotal.GetMetricWithLabelValues(controllerName.MetricsName(), label)
	require.NoError(t, err)

	metricProto := &promDTO.Metric{}

	err = errMetric.Write(metricProto)
	require.NoError(t, err)

	beforeCount := metricProto.GetCounter().GetValue()
	return beforeCount
}

func StartTestingEnv() (*rest.Config, *envtest.Environment, error) {
	env := &envtest.Environment{}
	restConfig, err := env.Start()
	if err != nil {
		return nil, nil, err
	}
	return restConfig, env, nil
}

func CleanupTestingEnv(env *envtest.Environment) error {
	return env.Stop()
}

func RegisterSchemes(t *testing.T, builder *fake.ClientBuilder, regFuncs ...func(s *runtime.Scheme) error) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	for _, regFunc := range regFuncs {
		require.NoError(t, regFunc(scheme))
	}

	return builder.WithScheme(scheme)
}

func NewTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(secv1.Install(s))
	utilruntime.Must(cfgv1alpha2.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(approutingv1alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))
	return s
}

func NewTestEnvironment() *envtest.Environment {
	return &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "gatewaycrd"),
		},
	}
}
