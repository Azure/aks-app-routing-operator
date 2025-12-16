package dns

import (
	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultDomainDNSResourceName = "default-domain-dns"

var name = controllername.New("default", "domain", "dns")

// NewDefaultDomainDNSReconciler creates a new reconciler for managing external DNS records for the default domain in the cluster.
func NewDefaultDomainDNSReconciler(
	manager ctrl.Manager,
	conf *config.Config,
) error {
	return common.NewResourceReconciler(
		manager,
		name,
		defaultDomainObjects(conf),
		reconcileInterval,
	)
}

func defaultDomainObjects(conf *config.Config) []client.Object {
	objs := []client.Object{
		defaultDomainServiceAccount(conf),
		defaultDomainClusterExternalDNS(conf),
	}

	// Can safely assume the namespace exists if using kube-system.
	// This will basically never happen but is a legacy thing for some tests and should
	// be kept for compatibility.
	if conf.NS != "kube-system" {
		objs = append([]client.Object{manifests.Namespace(conf, conf.NS)}, objs...) // put namespace at front, so we can create resources in order
	}

	return objs
}

func defaultDomainServiceAccount(conf *config.Config) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultDomainDNSResourceName,
			Namespace: conf.NS,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": conf.DefaultDomainClientID,
			},
			Labels: manifests.GetTopLevelLabels(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
	}
}

func defaultDomainClusterExternalDNS(conf *config.Config) *approutingv1alpha1.ClusterExternalDNS {
	return &approutingv1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultDomainDNSResourceName,
			Namespace: conf.NS,
			Labels:    manifests.GetTopLevelLabels(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: approutingv1alpha1.GroupVersion.String(),
		},
		Spec: approutingv1alpha1.ClusterExternalDNSSpec{
			ResourceName:       defaultDomainDNSResourceName,
			ResourceNamespace:  conf.NS,
			DNSZoneResourceIDs: []string{conf.DefaultDomainZoneID},
			ResourceTypes:      []string{"ingress", "gateway"},
			Identity: approutingv1alpha1.ExternalDNSIdentity{
				ServiceAccount: defaultDomainDNSResourceName,
			},
		},
	}
}
