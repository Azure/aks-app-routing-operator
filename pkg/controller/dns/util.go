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
	GetTenantId() *string
	GetInputServiceAccount() string
	GetResourceNamespace() string
	GetInputResourceName() string
	GetResourceTypes() []string
	GetDnsZoneresourceIDs() []string
	GetFilters() *v1alpha1.ExternalDNSFilters
	GetNamespaced() bool
	GetIdentity() v1alpha1.ExternalDNSIdentity
	client.Object
}

func buildInputDNSConfig(e ExternalDNSCRDConfiguration, config *config.Config) manifests.InputExternalDNSConfig {
	identity := e.GetIdentity()

	// Determine identity type
	identityType := manifests.IdentityTypeWorkloadIdentity
	var clientId string
	var serviceAccount string

	if identity.Type == v1alpha1.IdentityTypeManagedIdentity {
		identityType = manifests.IdentityTypeMSI
		clientId = identity.ClientID
		// For MSI, we generate a service account name based on the resource name
		serviceAccount = ""
	} else {
		serviceAccount = identity.ServiceAccount
	}

	ret := manifests.InputExternalDNSConfig{
		IdentityType:        identityType,
		ClientId:            clientId,
		InputServiceAccount: serviceAccount,
		Namespace:           e.GetResourceNamespace(),
		InputResourceName:   e.GetInputResourceName(),
		ResourceTypes:       extractResourceTypes(e.GetResourceTypes()),
		DnsZoneresourceIDs:  e.GetDnsZoneresourceIDs(),
		Filters:             e.GetFilters(),
		IsNamespaced:        e.GetNamespaced(),
		UID:                 string(e.GetUID()),
	}

	switch e.GetTenantId() {
	case nil:
		ret.TenantId = config.TenantID
	default:
		ret.TenantId = *e.GetTenantId()
	}

	return ret
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
	inputDNSConf := buildInputDNSConfig(obj, config)
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
		if resource.GetObjectKind().GroupVersionKind().Kind != "Namespace" { // don't want to set owner references in case we're generating the ns
			resource.SetOwnerReferences(owners)
		}
		currentResourceErr := util.Upsert(ctx, client, resource)
		multiError = multierror.Append(multiError, currentResourceErr)
	}

	return multiError.ErrorOrNil()
}

// verifyIdentity verifies that the identity configuration is valid for the ExternalDNS resource.
// For workload identity, it validates that the service account exists and has the required annotation.
// For managed identity, no additional verification is needed as the clientID is validated by CRD schema.
func verifyIdentity(ctx context.Context, k8sclient client.Client, obj ExternalDNSCRDConfiguration) error {
	identity := obj.GetIdentity()

	// For workload identity (or default/empty which defaults to workload identity),
	// verify the service account exists and has the required annotation
	if identity.Type != v1alpha1.IdentityTypeManagedIdentity {
		_, err := util.GetServiceAccountWorkloadIdentityClientId(ctx, k8sclient, identity.ServiceAccount, obj.GetResourceNamespace())
		return err
	}

	// For managed identity, the clientID is validated by CRD schema validation
	// No additional verification is needed at runtime
	return nil
}
