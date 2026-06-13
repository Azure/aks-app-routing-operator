package dns

import (
	"context"
	"fmt"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDomainDNSResourceName = "default-domain-dns"
	// defaultDomainServiceAccountName is the name of the service account used by the default domain external DNS controller.
	// This exact name must be used because aks rp federates credentials for this
	defaultDomainServiceAccountName = "default-domain-sa"
)

var name = controllername.New("default", "domain", "dns")

// CleanDefaultDomainDNS removes the resources created by the default domain DNS reconciler. It's used to tear down
// the default-domain-dns-external-dns Deployment (and its other resources) when the default domain feature is
// disabled. Deleting the default-domain-dns ClusterExternalDNS cascades to its owned Deployment via owner references,
// so without this cleanup the Deployment is orphaned and crash-loops once its DNS zone and identity RBAC are gone.
func CleanDefaultDomainDNS(ctx context.Context, c client.Client, conf *config.Config, lgr logr.Logger) error {
	lgr = name.AddToLogger(lgr)

	// Delete the ClusterExternalDNS first so Kubernetes garbage collection removes the owned Deployment/ConfigMap.
	// We intentionally don't delete the namespace since it's shared with other app routing resources.
	toDelete := []client.Object{
		&approutingv1alpha1.ClusterExternalDNS{
			ObjectMeta: metav1.ObjectMeta{Name: defaultDomainDNSResourceName, Namespace: conf.NS},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: defaultDomainServiceAccountName, Namespace: conf.NS},
		},
	}

	for _, obj := range toDelete {
		lgr := lgr.WithValues("name", obj.GetName(), "namespace", obj.GetNamespace())
		lgr.Info("deleting disabled default domain dns resource")
		if err := c.Delete(ctx, obj); err != nil {
			if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				continue
			}
			return fmt.Errorf("deleting default domain dns resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}

	return nil
}

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
	var objs []client.Object

	if conf.EnabledWorkloadIdentity {
		objs = append(objs, defaultDomainServiceAccount(conf))
	}

	objs = append(objs, defaultDomainClusterExternalDNS(conf))

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
			Name:      defaultDomainServiceAccountName,
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
	resourceTypes := []string{"ingress"}
	if conf.EnableDefaultDomainGateway {
		resourceTypes = append(resourceTypes, "gateway")
	}

	var identity approutingv1alpha1.ExternalDNSIdentity
	if conf.EnabledWorkloadIdentity {
		identity = approutingv1alpha1.ExternalDNSIdentity{
			Type:           approutingv1alpha1.IdentityTypeWorkloadIdentity,
			ServiceAccount: defaultDomainServiceAccountName,
		}
	} else {
		identity = approutingv1alpha1.ExternalDNSIdentity{
			Type:     approutingv1alpha1.IdentityTypeManagedIdentity,
			ClientID: conf.DefaultDomainClientID,
		}
	}

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
			ResourceTypes:      resourceTypes,
			Identity:           identity,
		},
	}
}
