package nginxingress

import (
	"context"
	"fmt"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultcName is the default Ingress class name
	DefaultIcName = "webapprouting.kubernetes.azure.com"
	// DefaultNicName is the default Nginx Ingress Controller resource name
	DefaultNicName         = "default"
	DefaultNicResourceName = "nginx"
	reconcileInterval      = time.Minute * 3
)

func NewDefaultReconciler(mgr ctrl.Manager) error {
	nic := &approutingv1alpha1.NginxIngressController{
		TypeMeta: metav1.TypeMeta{
			APIVersion: approutingv1alpha1.GroupVersion.String(),
			Kind:       "NginxIngressController",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicName,
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			ControllerNamePrefix: "nginx",
			IngressClassName:     DefaultIcName,
		},
		Status: approutingv1alpha1.NginxIngressControllerStatus{},
	}

	if err := common.NewResourceReconciler(mgr, controllername.New("default", "nginx", "ingress", "controller", "reconciler"), []client.Object{nic}, reconcileInterval); err != nil {
		return fmt.Errorf("creating default nginx ingress controller: %w", err)
	}

	return nil
}

func getDefaultIngressClassControllerClass(cl client.Client) (string, error) {
	defaultNicCc := "webapprouting.kubernetes.azure.com/nginx"

	defaultIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultIcName,
		},
	}
	err := cl.Get(context.Background(), types.NamespacedName{Name: DefaultIcName}, defaultIc)
	if err == nil {
		defaultNicCc = defaultIc.Spec.Controller
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", fmt.Errorf("getting default ingress class: %w", err)
	}

	return defaultNicCc, nil
}

// IsDefaultNic returns true if the given NginxIngressController is the default one
func IsDefaultNic(nic *approutingv1alpha1.NginxIngressController) bool {
	if nic == nil {
		return false
	}

	return nic.Name == DefaultNicName && nic.Spec.IngressClassName == DefaultIcName
}
