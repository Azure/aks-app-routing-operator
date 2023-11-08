package nginxingress

import (
	"context"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultNicReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	// when default nic doesn't exist in cluster we don't create the default nic
	d := &defaultNicReconciler{
		client: cl,
		lgr:    logr.Discard(),
		name:   controllername.New("testing"),
	}
	require.NoError(t, d.Start(context.Background()))

	nic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicName,
		},
	}
	require.True(t, k8serrors.IsNotFound(d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic)))

	// when default nic exists in cluster we create the default nic
	require.NoError(t, cl.Create(context.Background(), &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultIcName,
		},
	}))
	require.NoError(t, d.Start(context.Background()))
	require.NoError(t, d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic))
	require.Equal(t, "nginx", nic.Spec.ControllerNamePrefix)
	require.Equal(t, DefaultIcName, nic.Spec.IngressClassName)

}

func TestShouldCreateDefaultNic(t *testing.T) {
	cl := fake.NewClientBuilder().Build()

	// when default ic doesn't exist in cluster
	shouldCreate, err := shouldCreateDefaultNic(cl)
	require.NoError(t, err)
	require.False(t, shouldCreate)

	// when default ic exists in cluster
	require.NoError(t, cl.Create(context.Background(), &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultIcName,
		},
	}))
	shouldCreate, err = shouldCreateDefaultNic(cl)
	require.NoError(t, err)
	require.True(t, shouldCreate)
}

func TestGetDefaultIngressClassControllerClass(t *testing.T) {
	cl := fake.NewClientBuilder().Build()

	// when default IngressClass doesn't exist in cluster it defaults to webapprouting.kubernetes.azure.com/nginx
	cc, err := GetDefaultIngressClassControllerClass(cl)
	require.NoError(t, err)
	require.Equal(t, "webapprouting.kubernetes.azure.com/nginx", cc)

	// when IngressClass exists in cluster we take whatever is in the Spec.Controller field
	controller := "controllerField"
	ic := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultIcName,
		},
		Spec: netv1.IngressClassSpec{
			Controller: controller,
		},
	}
	require.NoError(t, cl.Create(context.Background(), ic))
	cc, err = GetDefaultIngressClassControllerClass(cl)
	require.NoError(t, err)
	require.Equal(t, controller, cc)
}

func TestIsDefaultNic(t *testing.T) {
	cases := []struct {
		Name     string
		Nic      *approutingv1alpha1.NginxIngressController
		Expected bool
	}{
		{
			Name:     "nil nic",
			Nic:      nil,
			Expected: false,
		},
		{
			Name: "default name, default IngressClassName",
			Nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: DefaultIcName,
				},
			},
			Expected: true,
		},
		{
			Name: "default name, non default IngressClassName",
			Nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: "non-default",
				},
			},
			Expected: false,
		},
		{
			Name: "non default name, default IngressClassName",
			Nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-default",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: DefaultIcName,
				},
			},
			Expected: false,
		},
		{
			Name: "non default name, non default IngressClassName",
			Nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-default",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName: "non-default",
				},
			},
			Expected: false,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			require.Equal(t, c.Expected, IsDefaultNic(c.Nic))
		})
	}
}

func TestGetDefaultNginxIngressController(t *testing.T) {
	ret := GetDefaultNginxIngressController()
	require.NotNil(t, ret)
	require.Equal(t, DefaultNicName, ret.Name)
	require.Equal(t, DefaultIcName, ret.Spec.IngressClassName)
	require.Equal(t, DefaultNicResourceName, ret.Spec.ControllerNamePrefix)
}
