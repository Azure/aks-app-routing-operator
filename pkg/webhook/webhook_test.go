package webhook

import (
	"context"
	"errors"
	"testing"

	globalCfg "github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNew(t *testing.T) {
	cases := []struct {
		name     string
		global   *globalCfg.Config
		expected *config
		err      error
	}{
		{
			name:     "nil config",
			global:   nil,
			expected: nil,
			err:      errors.New("config is nil"),
		},
		{
			name: "valid config",
			global: &globalCfg.Config{
				OperatorWebhookService: "service-name",
				OperatorNs:             "namespace",
				WebhookPort:            443,
				CertDir:                "cert-dir",
			},
			expected: &config{
				serviceName:                 "service-name",
				namespace:                   "namespace",
				port:                        443,
				certDir:                     "cert-dir",
				validatingWebhookConfigName: "app-routing-validating",
				mutatingWebhookConfigName:   "app-routing-mutating",
				validatingWebhooks:          Validating,
				mutatingWebhooks:            Mutating,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := New(c.global)
			require.Equal(t, c.err, err, "unexpected error")
			if err != nil {
				return
			}

			if actual.serviceName != c.expected.serviceName {
				t.Errorf("expected service name %s, got %s", c.expected.serviceName, actual.serviceName)
			}
			if actual.namespace != c.expected.namespace {
				t.Errorf("expected namespace %s, got %s", c.expected.namespace, actual.namespace)
			}
			if actual.port != c.expected.port {
				t.Errorf("expected port %d, got %d", c.expected.port, actual.port)
			}
			if actual.certDir != c.expected.certDir {
				t.Errorf("expected cert dir %s, got %s", c.expected.certDir, actual.certDir)
			}
			if actual.validatingWebhookConfigName != c.expected.validatingWebhookConfigName {
				t.Errorf("expected validating webhook config name %s, got %s", c.expected.validatingWebhookConfigName, actual.validatingWebhookConfigName)
			}
			if actual.mutatingWebhookConfigName != c.expected.mutatingWebhookConfigName {
				t.Errorf("expected mutating webhook config name %s, got %s", c.expected.mutatingWebhookConfigName, actual.mutatingWebhookConfigName)
			}
			if len(actual.validatingWebhooks) != len(c.expected.validatingWebhooks) {
				t.Errorf("expected %d validating webhooks, got %d", len(c.expected.validatingWebhooks), len(actual.validatingWebhooks))
			}
			if len(actual.mutatingWebhooks) != len(c.expected.mutatingWebhooks) {
				t.Errorf("expected %d mutating webhooks, got %d", len(c.expected.mutatingWebhooks), len(actual.mutatingWebhooks))
			}
		})
	}
}

func TestEnsureWebhookConfigurations(t *testing.T) {
	t.Run("valid webhooks", func(t *testing.T) {
		globalCfg := &globalCfg.Config{
			NS: "app-routing-system",
		}
		c := &config{
			validatingWebhookConfigName: "app-routing-validating",
			mutatingWebhookConfigName:   "app-routing-mutating",
			validatingWebhooks:          Validating,
			mutatingWebhooks:            Mutating,
			certDir: 				   "testcerts",
			caName: 				   "ca.crt",
		}

		cl := fake.NewClientBuilder().Build()
		var validatingWhCfg admissionregistrationv1.ValidatingWebhookConfiguration
		var mutatingWhCfg admissionregistrationv1.MutatingWebhookConfiguration
		// prove webhook configurations don't exist
		require.True(t, k8serrors.IsNotFound(cl.Get(context.Background(), types.NamespacedName{Name: c.validatingWebhookConfigName}, &validatingWhCfg)), "expected not to find validating webhook config")
		require.True(t, k8serrors.IsNotFound(cl.Get(context.Background(), types.NamespacedName{Name: c.mutatingWebhookConfigName}, &mutatingWhCfg)), "expected not to find mutating webhook config")

		// prove webhook configurations exist after ensuring
		require.NoError(t, c.EnsureWebhookConfigurations(context.Background(), cl, globalCfg), "unexpected error")
		require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: c.validatingWebhookConfigName}, &validatingWhCfg), "unexpected error getting validating webhook config")
		require.NoError(t, cl.Get(context.Background(), types.NamespacedName{Name: c.mutatingWebhookConfigName}, &mutatingWhCfg), "unexpected error getting mutating webhook config")

		// prove ownerReferences are set on webhook configurations
		require.True(t, len(validatingWhCfg.OwnerReferences) == 1, "expected 1 owner reference")
		require.True(t, validatingWhCfg.OwnerReferences[0].Name == "app-routing-system", "expected owner reference name to be app-routing-system")
		require.True(t, validatingWhCfg.OwnerReferences[0].Kind == "Namespace", "expected owner reference kind to be Namespace")
		require.True(t, validatingWhCfg.OwnerReferences[0].APIVersion == "v1", "expected owner reference api version to be v1")
		require.True(t, len(mutatingWhCfg.OwnerReferences) == 1, "expected 1 owner reference")
		require.True(t, mutatingWhCfg.OwnerReferences[0].Name == "app-routing-system", "expected owner reference name to be app-routing-system")
		require.True(t, mutatingWhCfg.OwnerReferences[0].Kind == "Namespace", "expected owner reference kind to be Namespace")
		require.True(t, mutatingWhCfg.OwnerReferences[0].APIVersion == "v1", "expected owner reference api version to be v1")

	})

	t.Run("invalid webhooks", func(t *testing.T) {

		globalCfg := &globalCfg.Config{}
		c := &config{
			validatingWebhooks: []Webhook[admissionregistrationv1.ValidatingWebhook]{
				{
					Definition: func(c *config) (admissionregistrationv1.ValidatingWebhook, error) {
						return admissionregistrationv1.ValidatingWebhook{}, errors.New("invalid webhook")
					},
				},
			},
			mutatingWebhooks: []Webhook[admissionregistrationv1.MutatingWebhook]{
				{
					Definition: func(c *config) (admissionregistrationv1.MutatingWebhook, error) {
						return admissionregistrationv1.MutatingWebhook{}, errors.New("invalid webhook")
					},
				},
			},
		}

		cl := fake.NewClientBuilder().Build()
		require.True(t, c.EnsureWebhookConfigurations(context.Background(), cl, globalCfg).Error() == "getting webhook definition: invalid webhook", "expected error")

		c.validatingWebhooks = nil
		require.True(t, c.EnsureWebhookConfigurations(context.Background(), cl, globalCfg).Error() == "getting webhook definition: invalid webhook", "expected error")
	})
}

func TestGetClientConfig(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		cfg      *config
		expected admissionregistrationv1.WebhookClientConfig
		err      error
	}{
		{
			name: "basic names",
			cfg: &config{
				serviceName: "service-name",
				namespace:   "namespace",
				port:        443,
				certDir:     "testcerts",
				caName:      "ca.crt",
			},
			path: "/example-path",
			expected: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name:      "service-name",
					Namespace: "namespace",
					Path:      util.ToPtr("/example-path"),
					Port:      util.ToPtr(int32(443)),
				},
			},
			err: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := c.cfg.GetClientConfig(c.path)
			if err != c.err {
				t.Errorf("unexpected error: %v", err)
			}

			if actual.Service.Name != c.expected.Service.Name {
				t.Errorf("expected service name %s, got %s", c.expected.Service.Name, actual.Service.Name)
			}
			if actual.Service.Namespace != c.expected.Service.Namespace {
				t.Errorf("expected service namespace %s, got %s", c.expected.Service.Namespace, actual.Service.Namespace)
			}
			if *actual.Service.Port != *c.expected.Service.Port {
				t.Errorf("expected service port %d, got %d", *c.expected.Service.Port, *actual.Service.Port)
			}
			if *actual.Service.Path != *c.expected.Service.Path {
				t.Errorf("expected service path %s, got %s", *c.expected.Service.Path, *actual.Service.Path)
			}
		})
	}
}
