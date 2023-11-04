package nginxingress

import (
	"context"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDefaultIngressClassControllerClass(t *testing.T) {
	cl := fake.NewClientBuilder().Build()

	// when default IngressClass doesn't exist in cluster it defaults to webapprouting.kubernetes.azure.com/nginx
	cc, err := getDefaultIngressClassControllerClass(cl)
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
	cc, err = getDefaultIngressClassControllerClass(cl)
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