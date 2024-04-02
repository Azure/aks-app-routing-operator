package nginxingress

import (
	"context"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
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

	// prove we always create the default nic
	d := &defaultNicReconciler{
		client: cl,
		lgr:    logr.Discard(),
		name:   controllername.New("testing"),
		conf:   config.Config{},
	}

	nic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicName,
		},
	}
	require.NoError(t, d.tick(context.Background()))
	require.NoError(t, d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic))
	require.Equal(t, "nginx", nic.Spec.ControllerNamePrefix)
	require.Equal(t, DefaultIcName, nic.Spec.IngressClassName)

	// prove there are no default nic lb service annotations
	require.Equal(t, *new(map[string]string), nic.Spec.LoadBalancerAnnotations, "default nic service annotations should be empty initially")
	// prove we don't overwrite the default nic lb service annotations
	newLbAnnotations := map[string]string{"foo": "bar"}
	nic.Spec.LoadBalancerAnnotations = newLbAnnotations
	require.NoError(t, d.client.Update(context.Background(), nic))
	require.NoError(t, d.tick(context.Background()))
	require.NoError(t, d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic))
	require.Equal(t, newLbAnnotations, nic.Spec.LoadBalancerAnnotations, "default nic service annotations should not be overwritten")
	// clear old arbitrary annotations
	nic.Spec.LoadBalancerAnnotations = map[string]string{}
	require.NoError(t, d.client.Update(context.Background(), nic))

	// prove that a private nic lb service annotation is used when the configuration specifies
	d.conf.DefaultController = config.Private
	require.NoError(t, d.tick(context.Background()))
	require.NoError(t, d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic))
	require.Equal(t, map[string]string{internalLbAnnotation: "true"}, nic.Spec.LoadBalancerAnnotations, "default nic service annotations should have private lb annotation")

	// prove that a public nic lb service annotation is used when the configuration specifies
	d.conf.DefaultController = config.Public
	require.NoError(t, d.tick(context.Background()))
	require.NoError(t, d.client.Get(context.Background(), types.NamespacedName{Name: nic.Name}, nic))
	require.Equal(t, map[string]string{internalLbAnnotation: "false"}, nic.Spec.LoadBalancerAnnotations, "default nic service annotations should have private lb annotation")
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
