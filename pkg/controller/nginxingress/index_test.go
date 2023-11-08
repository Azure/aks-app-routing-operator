package nginxingress

import (
	"context"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressClassNameIndexFn(t *testing.T) {
	cases := []struct {
		name     string
		object   *approutingv1alpha1.NginxIngressController
		expected []string
	}{
		{
			name:     "nil spec",
			object:   &approutingv1alpha1.NginxIngressController{},
			expected: []string{""},
		},
		{
			name: "standard spec",
			object: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: "foo",
				},
			},
			expected: []string{"foo"},
		},
		{
			name: "another standard spec",
			object: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: "bar",
				},
			},
			expected: []string{"bar"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ingressClassNameIndexFn(tc.object))
		})
	}
}

func TestIsIngressManaged(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	idxName := "testIndex"
	cl := fake.NewClientBuilder().WithScheme(scheme).WithIndex(
		&approutingv1alpha1.NginxIngressController{}, idxName, ingressClassNameIndexFn,
	).Build()
	ctx := context.Background()

	// no existing nics
	managed, err := IsIngressManaged(ctx, cl, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("icName"),
		},
	}, idxName)
	require.NoError(t, err)
	require.False(t, managed)

	// default nic
	managed, err = IsIngressManaged(ctx, cl, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr(DefaultIcName),
		},
	}, idxName)
	require.NoError(t, err)
	require.True(t, managed)

	// existing nic
	existingNic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "fooIngressClass",
			ControllerNamePrefix: "foo",
		},
	}
	require.NoError(t, cl.Create(ctx, existingNic))

	managed, err = IsIngressManaged(ctx, cl, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("fooIngressClass"),
		},
	}, idxName)
	require.NoError(t, err)
	require.True(t, managed)

	managed, err = IsIngressManaged(ctx, cl, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("otherIngressClass"),
		},
	}, idxName)
	require.NoError(t, err)
	require.False(t, managed)
}

func TestIngressSource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	idxName := "testIndex"
	cl := fake.NewClientBuilder().WithScheme(scheme).WithIndex(
		&approutingv1alpha1.NginxIngressController{}, idxName, ingressClassNameIndexFn,
	).Build()
	ctx := context.Background()
	defaultCc := "defaultControllerClass"
	conf := &config.Config{NS: "defaultNs"}

	// no ingressClassName
	source, managed, err := IngressSource(ctx, cl, conf, defaultCc, &netv1.Ingress{
		Spec: netv1.IngressSpec{},
	}, idxName)
	require.NoError(t, err)
	require.Equal(t, policyv1alpha1.IngressSourceSpec{}, source)
	require.False(t, managed)

	// default ingressClassName
	source, managed, err = IngressSource(ctx, cl, conf, defaultCc, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr(DefaultIcName),
		},
	}, idxName)
	require.NoError(t, err)
	require.Equal(t, policyv1alpha1.IngressSourceSpec{
		Kind:      "Service",
		Name:      DefaultNicResourceName,
		Namespace: conf.NS,
	}, source)
	require.True(t, managed)

	// unmanaged ingressClassName
	source, managed, err = IngressSource(ctx, cl, conf, defaultCc, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("otherIngressClass"),
		},
	}, idxName)
	require.NoError(t, err)
	require.Equal(t, policyv1alpha1.IngressSourceSpec{}, source)
	require.False(t, managed)

	// managed
	nic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "fooIngressClass",
			ControllerNamePrefix: "foo",
		},
	}
	require.NoError(t, cl.Create(ctx, nic))
	source, managed, err = IngressSource(ctx, cl, conf, defaultCc, &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("fooIngressClass"),
		},
	}, idxName)
	require.NoError(t, err)
	require.Equal(t, policyv1alpha1.IngressSourceSpec{
		Kind:      "Service",
		Name:      "foo-0",
		Namespace: conf.NS,
	}, source)
	require.True(t, managed)
}
