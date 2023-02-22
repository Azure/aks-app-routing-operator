package dns

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 3

// newExternalDnsReconciler creates a reconciler that manages external dns resources
func newExternalDnsReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "externalDnsReconciler", resources, reconcileInterval)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	if conf.DNSZoneDomain == "" {
		return nil
	}

	dnsConfig := &manifests.ExternalDnsConfig{
		ResourceName:  "external-dns",
		TenantId:      conf.TenantID,
		Subscription:  conf.DNSZoneSub,
		ResourceGroup: conf.DNSZoneRG,
		Domain:        conf.DNSZoneDomain,
		RecordId:      conf.DNSRecordID,
		IsPrivate:     conf.DNSZonePrivate,
	}
	objs := manifests.ExternalDnsResources(conf, self, dnsConfig)
	return newExternalDnsReconciler(manager, objs)
}
