package dns

import (
	"context"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/hashicorp/go-multierror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExternalDNSCRDConfiguration interface {
	GetTenantId() string
	GetInputServiceAccount() string
	GetResourceNamespace() string
	GetInputResourceName() string
	GetResourceTypes() []string
	GetDnsZoneresourceIDs() []string
	GetFilters() *v1alpha1.ExternalDNSFilters
	GetNamespaced() bool
}

func buildInputDNSConfig(e ExternalDNSCRDConfiguration) manifests.InputExternalDNSConfig {
	return manifests.InputExternalDNSConfig{
		IdentityType:        manifests.IdentityTypeWorkloadIdentity,
		TenantId:            e.GetTenantId(),
		InputServiceAccount: e.GetInputServiceAccount(),
		Namespace:           e.GetResourceNamespace(),
		InputResourceName:   e.GetInputResourceName(),
		ResourceTypes:       extractResourceTypes(e.GetResourceTypes()),
		DnsZoneresourceIDs:  e.GetDnsZoneresourceIDs(),
		Filters:             e.GetFilters(),
		IsNamespaced:        e.GetNamespaced(),
	}
}

func extractResourceTypes(resourceTypes []string) map[manifests.ResourceType]struct{} {
	ret := map[manifests.ResourceType]struct{}{}
	for _, rt := range resourceTypes {
		if strings.EqualFold(rt, manifests.ResourceTypeIngress.String()) {
			ret[manifests.ResourceTypeIngress] = struct{}{}
		}
		if strings.EqualFold(rt, manifests.ResourceTypeGateway.String()) {
			ret[manifests.ResourceTypeGateway] = struct{}{}
		}
	}

	return ret
}

func generateManifestsConf(config *config.Config, obj ExternalDNSCRDConfiguration) (*manifests.ExternalDnsConfig, error) {
	inputDNSConf := buildInputDNSConfig(obj)
	manifestsConf, err := manifests.NewExternalDNSConfig(config, inputDNSConf)
	if err != nil {
		return nil, util.NewUserError(err, "failed to generate ExternalDNS resources: "+err.Error())
	}

	return manifestsConf, nil
}

func deployExternalDNSResources(ctx context.Context, client client.Client, manifestsConf *manifests.ExternalDnsConfig, owners []metav1.OwnerReference) error {
	// create the ExternalDNS resources
	multiError := &multierror.Error{}

	for _, resource := range manifestsConf.Resources() {
		resource.SetOwnerReferences(owners)

		currentResourceErr := util.Upsert(ctx, client, resource)
		multiError = multierror.Append(multiError, currentResourceErr)
	}

	return multiError.ErrorOrNil()
}
