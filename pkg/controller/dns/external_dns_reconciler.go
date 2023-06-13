package dns

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	reconcileInterval = time.Minute * 3
)

// newExternalDnsReconciler creates a reconciler that manages external dns resources
func newExternalDnsReconciler(manager ctrl.Manager, resources []client.Object) error {
	return common.NewResourceReconciler(manager, "externalDnsReconciler", resources, reconcileInterval)
}

// NewExternalDns starts all resources required for external dns
func NewExternalDns(manager ctrl.Manager, conf *config.Config, self *appsv1.Deployment) error {
	if conf.PublicZoneConfig != nil && len(conf.PublicZoneConfig.ZoneIds) == 0 && conf.PrivateZoneConfig != nil && len(conf.PrivateZoneConfig.ZoneIds) == 0 {
		return nil
	}

	clusterIdentifier := getClusterIdentifierOrExternalDNS(context.Background(), manager)
	configs, _ := generateZoneConfigs(conf, clusterIdentifier)

	// TODO: Uncomment this to implement an externalDNS cleanup runner
	//err := newCleanupRunner(manager, namesToDelete)
	//
	//if err != nil {
	//	return fmt.Errorf("failed to start cleanup runner: %w", err)
	//}

	objs := append([]client.Object{}, manifests.ExternalDnsResources(conf, self, configs)...)

	return newExternalDnsReconciler(manager, objs)
}

func generateZoneConfigs(conf *config.Config, clusterIdentifier string) (configs []*manifests.ExternalDnsConfig, namesToDelete []string) {
	publicResourceName := fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PublicSuffix)
	privateResourceName := fmt.Sprintf("%s%s", manifests.ExternalDnsResourceName, manifests.PrivateSuffix)

	if conf.PrivateZoneConfig != nil && len(conf.PrivateZoneConfig.ZoneIds) > 0 {
		configs = append(configs, generateConfig(conf, conf.PrivateZoneConfig, manifests.PrivateProvider, privateResourceName, clusterIdentifier))
	} else {
		namesToDelete = append(namesToDelete, privateResourceName)
	}

	if conf.PublicZoneConfig != nil && len(conf.PublicZoneConfig.ZoneIds) > 0 {
		configs = append(configs, generateConfig(conf, conf.PublicZoneConfig, manifests.PublicProvider, publicResourceName, clusterIdentifier))
	} else {
		namesToDelete = append(namesToDelete, publicResourceName)
	}

	// namesToDelete will eventually be used for externalDNS cleanup
	return
}

func generateConfig(conf *config.Config, dnsZoneConfig *config.DnsZoneConfig, provider manifests.Provider, resourceName, clusterIdentifier string) *manifests.ExternalDnsConfig {
	return &manifests.ExternalDnsConfig{
		ResourceName:       resourceName,
		TenantId:           conf.TenantID,
		Subscription:       dnsZoneConfig.Subscription,
		ResourceGroup:      dnsZoneConfig.ResourceGroup,
		DnsZoneResourceIDs: dnsZoneConfig.ZoneIds,
		Provider:           provider,
		ClusterIdentifier:  clusterIdentifier,
	}
}

func getClusterIdentifierOrExternalDNS(ctx context.Context, manager ctrl.Manager) string {
	// convention for unique identifier is to use UID of kube-system namespace
	// see: https://github.com/kubernetes/kubernetes/issues/77487#issuecomment-489786023
	c := manager.GetClient()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	err := c.Get(ctx, client.ObjectKey{Name: "kube-system"}, ns)
	if err != nil || ns.UID == "" {
		log.Printf("WARNING: failed to get namespace kube-system: %s, using externalDNS as dns record owner", err)
		return manifests.ExternalDnsResourceName
	}

	return string(ns.UID)
}
