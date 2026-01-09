package manifests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"k8s.io/apimachinery/pkg/runtime/schema"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	replicas                = 1 // this must stay at 1 unless external-dns adds support for multiple replicas https://github.com/kubernetes-sigs/external-dns/issues/2430
	k8sNameKey              = "app.kubernetes.io/name"
	externalDnsResourceName = "external-dns"
	txtWildcardReplacement  = "approutingwildcard"
)

type IdentityType int

const (
	IdentityTypeMSI IdentityType = iota
	IdentityTypeWorkloadIdentity
)

func (i IdentityType) externalDNSIdentityConfiguration() string {
	switch i {
	case IdentityTypeWorkloadIdentity:
		return "useWorkloadIdentityExtension"
	default:
		return "useManagedIdentityExtension"
	}
}

type ResourceType int

const (
	ResourceTypeIngress ResourceType = iota
	ResourceTypeGateway

	maxUIDLength   = 16
	checkSumLength = 16
)

func (rt ResourceType) String() string {
	switch rt {
	case ResourceTypeGateway:
		return "Gateway"
	default:
		return "Ingress"
	}
}

func (rt ResourceType) generateResourceDeploymentArgs() []string {
	switch rt {
	case ResourceTypeGateway:
		return []string{
			"--source=gateway-httproute",
			"--source=gateway-grpcroute",
		}
	default:
		return []string{"--source=ingress"}
	}
}

func (rt ResourceType) generateRBACRules(dnsconfig *ExternalDnsConfig) []rbacv1.PolicyRule {
	switch rt {
	case ResourceTypeGateway:
		ret := []rbacv1.PolicyRule{
			{
				APIGroups: []string{"gateway.networking.k8s.io"},
				Resources: []string{"gateways", "httproutes", "grpcroutes"},
				Verbs:     []string{"get", "watch", "list"},
			},
		}

		return ret
	default:
		return []rbacv1.PolicyRule{
			{
				APIGroups: []string{"extensions", "networking.k8s.io"},
				Resources: []string{"ingresses"},
				Verbs:     []string{"get", "watch", "list"},
			},
		}
	}
}

// OldExternalDnsGks is a slice of GroupKinds that were previously used by ExternalDns.
// If the manifests used by app routing's external dns removes a GroupKind be sure to add
// it here to clean it up
var OldExternalDnsGks []schema.GroupKind

type Provider int

const (
	PublicProvider Provider = iota
	PrivateProvider
)

func (p Provider) string() string {
	switch p {
	case PublicProvider:
		return "azure"
	case PrivateProvider:
		return "azure-private-dns"
	default:
		return ""
	}
}

// InputExternalDNSConfig is the input configuration to generate ExternalDNSConfigs from the CRD or MC-level configuration
type InputExternalDNSConfig struct {
	TenantId, ClientId, InputServiceAccount, Namespace, InputResourceName string
	// Provider is specified when an InputConfig is coming from the MC External DNS Reconciler, since no zones may be provided for the clean case
	Provider *Provider
	// IdentityType can either be MSI or WorkloadIdentity
	IdentityType IdentityType
	// ResourceTypes refer to the resource types that ExternalDNS should look for to configure DNS. These can include Gateway and/or Ingress
	ResourceTypes map[ResourceType]struct{}
	// DnsZoneresourceIDs contains the DNS zones that ExternalDNS will use to configure DNS
	DnsZoneresourceIDs []string
	// Filters contains various filters that ExternalDNS will use to filter resources it scans for DNS configuration
	Filters *v1alpha1.ExternalDNSFilters
	// IsNamespaced is true if the ExternalDNS deployment should only scan for resources in the resource namespace, and false if it should scan all namespaces
	IsNamespaced bool
	// UID is an optional unique identifier to append to resource names to avoid conflicts
	UID string
}

// ExternalDnsConfig contains externaldns resources based on input configuration
type ExternalDnsConfig struct {
	// internally exposed
	tenantId, subscription, resourceGroup,
	clientId, serviceAccountName, namespace,
	resourceName string
	identityType  IdentityType
	resourceTypes map[ResourceType]struct{}
	provider      Provider
	isNamespaced  bool

	// crd-specific specific fields
	routeAndIngressLabelSelector string
	gatewayLabelSelector         string
	uid                          string

	// externally exposed
	resources          []client.Object
	labels             map[string]string
	dnsZoneResourceIDs []string
}

func (e *ExternalDnsConfig) Resources() []client.Object {
	return e.resources
}

func (e *ExternalDnsConfig) Labels() map[string]string {
	return e.labels
}

func (e *ExternalDnsConfig) DnsZoneResourceIds() []string {
	return e.dnsZoneResourceIDs
}

func NewExternalDNSConfig(conf *config.Config, inputConfig InputExternalDNSConfig) (*ExternalDnsConfig, error) {
	// valid values for enums
	if inputConfig.IdentityType != IdentityTypeMSI && inputConfig.IdentityType != IdentityTypeWorkloadIdentity {
		return nil, fmt.Errorf("invalid identity type: %v", inputConfig.IdentityType)
	}

	_, containsGateway := inputConfig.ResourceTypes[ResourceTypeGateway]
	if containsGateway && inputConfig.IdentityType != IdentityTypeWorkloadIdentity {
		return nil, errors.New("gateway resource type can only be used with workload identity")
	}

	var firstZoneResourceType string
	var firstZoneSub string
	var firstZoneRg string
	var provider Provider

	if len(inputConfig.DnsZoneresourceIDs) > 0 {
		firstZone, err := azure.ParseResourceID(inputConfig.DnsZoneresourceIDs[0])
		if err != nil {
			return nil, fmt.Errorf("invalid dns zone resource id: %s", inputConfig.DnsZoneresourceIDs[0])
		}

		firstZoneResourceType = firstZone.ResourceType
		firstZoneSub = firstZone.SubscriptionID
		firstZoneRg = firstZone.ResourceGroup

		// for some reason this passes tests without the if condition when arr has len 0 or 1, but I still feel weird about not having it
		if len(inputConfig.DnsZoneresourceIDs) > 1 {
			for _, zone := range inputConfig.DnsZoneresourceIDs[1:] {
				parsedZone, err := azure.ParseResourceID(zone)
				if err != nil {
					return nil, fmt.Errorf("invalid dns zone resource id: %s", zone)
				}

				if !strings.EqualFold(parsedZone.ResourceType, firstZoneResourceType) {
					return nil, fmt.Errorf("all DNS zones must be of the same type, found zones with resourcetypes %s and %s", firstZoneResourceType, parsedZone.ResourceType)
				}

				if err := config.ValidateProviderSubAndRg(parsedZone, firstZoneSub, firstZoneRg); err != nil {
					return nil, err
				}
			}
		}

		switch strings.ToLower(firstZoneResourceType) {
		case config.PrivateZoneType:
			provider = PrivateProvider
		case config.PublicZoneType:
			provider = PublicProvider
		default:
			return nil, fmt.Errorf("invalid resource type %s", firstZoneResourceType)
		}
	} else {
		// if no zones provided, this must be coming from the original externalDNS reconciler, in which case, read config from input to determine resources to clean
		if inputConfig.Provider == nil {
			return nil, errors.New("provider must be specified via inputconfig if no DNS zones are provided")
		}
		provider = *inputConfig.Provider
	}

	var resourceName string
	switch inputConfig.InputResourceName {
	case "":
		switch provider {
		case PrivateProvider:
			resourceName = externalDnsResourceName + "-private"
		default:
			resourceName = externalDnsResourceName
		}
	default:
		resourceName = inputConfig.InputResourceName + "-" + externalDnsResourceName
	}

	var cleanUID string
	if inputConfig.IsNamespaced && inputConfig.UID == "" {
		return nil, errors.New("namespaced external dns requires a unique identifier to avoid resource name conflicts")
	}

	cleanUID = strings.ReplaceAll(inputConfig.UID, "-", "")
	cleanUID = cleanUID[:int(math.Min(float64(len(cleanUID)), maxUIDLength))]

	if inputConfig.IdentityType == IdentityTypeWorkloadIdentity && inputConfig.InputServiceAccount == "" {
		return nil, errors.New("workload identity requires a service account name")
	}

	var serviceAccount string
	switch inputConfig.IdentityType {
	case IdentityTypeWorkloadIdentity:
		serviceAccount = inputConfig.InputServiceAccount
	default:
		serviceAccount = resourceName
	}

	ret := &ExternalDnsConfig{
		resourceName:       resourceName,
		tenantId:           inputConfig.TenantId,
		subscription:       firstZoneSub,
		resourceGroup:      firstZoneRg,
		clientId:           inputConfig.ClientId,
		serviceAccountName: serviceAccount,
		namespace:          inputConfig.Namespace,
		identityType:       inputConfig.IdentityType,
		resourceTypes:      inputConfig.ResourceTypes,
		provider:           provider,
		dnsZoneResourceIDs: inputConfig.DnsZoneresourceIDs,
		isNamespaced:       inputConfig.IsNamespaced,
		uid:                cleanUID,
	}

	if inputConfig.Filters != nil {
		gatewayLabel, err := parseLabel(inputConfig.Filters.GatewayLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parsing gateway label selector: %w", err)
		}

		routeAndIngressLabel, err := parseLabel(inputConfig.Filters.RouteAndIngressLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parsing route and ingress label selector: %w", err)
		}

		ret.gatewayLabelSelector = gatewayLabel
		ret.routeAndIngressLabelSelector = routeAndIngressLabel
	}

	ret.resources = externalDnsResources(conf, []*ExternalDnsConfig{ret})
	ret.labels = externalDNSLabels(ret)

	return ret, nil
}

func parseLabel(filterString *string) (string, error) {
	if filterString == nil || *filterString == "" {
		return "", nil
	}

	parts := strings.Split(*filterString, "=")

	if len(parts) != 2 {
		return "", fmt.Errorf("invalid label selector format: %s", *filterString)
	}

	return parts[0] + "==" + parts[1], nil
}

func externalDNSLabels(e *ExternalDnsConfig) map[string]string {
	labels := map[string]string{
		k8sNameKey: e.resourceName,
	}
	return labels
}

// externalDnsResources returns Kubernetes objects required for external dns
func externalDnsResources(conf *config.Config, externalDnsConfigs []*ExternalDnsConfig) []client.Object {
	var objs []client.Object
	namespaces := make(map[string]bool)
	for _, dnsConfig := range externalDnsConfigs {
		// Can safely assume the namespace exists if using kube-system
		if _, seen := namespaces[dnsConfig.namespace]; dnsConfig.namespace != "" && dnsConfig.namespace != "kube-system" && !seen {
			namespaces[dnsConfig.namespace] = true
			objs = append(objs, Namespace(conf, dnsConfig.namespace))
		}
		objs = append(objs, externalDnsResourcesFromConfig(conf, dnsConfig)...)
	}

	return objs
}

func externalDnsResourcesFromConfig(conf *config.Config, externalDnsConfig *ExternalDnsConfig) []client.Object {
	var objs []client.Object
	if externalDnsConfig.identityType == IdentityTypeMSI {
		objs = append(objs, newExternalDNSServiceAccount(externalDnsConfig))
	}

	if externalDnsConfig.isNamespaced {
		objs = append(objs, newExternalDnsNamespacedRBAC(externalDnsConfig)...)
	} else {
		objs = append(objs, newExternalDNSClusterRBAC(externalDnsConfig)...)
	}

	dnsCm, dnsCmHash := newExternalDNSConfigMap(conf, externalDnsConfig)
	objs = append(objs, dnsCm)
	objs = append(objs, newExternalDNSDeployment(conf, externalDnsConfig, dnsCmHash))

	for _, obj := range objs {
		l := util.MergeMaps(obj.GetLabels(), externalDNSLabels(externalDnsConfig))
		obj.SetLabels(l)
	}

	return objs
}

func newExternalDNSServiceAccount(externalDnsConfig *ExternalDnsConfig) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName,
			Namespace: externalDnsConfig.namespace,
			Labels:    GetTopLevelLabels(),
		},
	}
}

func newExternalDnsNamespacedRBAC(externalDnsConfig *ExternalDnsConfig) []client.Object {
	ret := []client.Object{}
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName,
			Namespace: externalDnsConfig.namespace,
			Labels:    GetTopLevelLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}

	// sort for fixture tests
	sortedRts := make([]ResourceType, 0, len(externalDnsConfig.resourceTypes))
	for resourceType := range externalDnsConfig.resourceTypes {
		sortedRts = append(sortedRts, resourceType)
	}
	sort.Slice(sortedRts, func(i, j int) bool { return sortedRts[i] < sortedRts[j] })
	for _, resourceType := range sortedRts {
		if resourceType == ResourceTypeGateway {
			ret = append(ret, listNamespaceRBAC(externalDnsConfig)...)
		}
		role.Rules = append(role.Rules, resourceType.generateRBACRules(externalDnsConfig)...)
	}

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName,
			Namespace: externalDnsConfig.namespace,
			Labels:    GetTopLevelLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     externalDnsConfig.resourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      externalDnsConfig.serviceAccountName,
			Namespace: externalDnsConfig.namespace,
		}},
	}

	return append([]client.Object{role, roleBinding}, ret...)
}

func newExternalDNSClusterRBAC(externalDnsConfig *ExternalDnsConfig) []client.Object {
	ret := []client.Object{}
	clusterRole := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName,
			Labels: GetTopLevelLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services", "configmaps"},
				Verbs:     []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}

	// sort for fixture tests
	sortedRts := make([]ResourceType, 0, len(externalDnsConfig.resourceTypes))
	for resourceType := range externalDnsConfig.resourceTypes {
		sortedRts = append(sortedRts, resourceType)
	}
	sort.Slice(sortedRts, func(i, j int) bool { return sortedRts[i] < sortedRts[j] })
	for _, resourceType := range sortedRts {
		clusterRole.Rules = append(clusterRole.Rules, resourceType.generateRBACRules(externalDnsConfig)...)
		if resourceType == ResourceTypeGateway {
			ret = append(ret, listNamespaceRBAC(externalDnsConfig)...)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName,
			Labels: GetTopLevelLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     externalDnsConfig.resourceName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      externalDnsConfig.serviceAccountName,
			Namespace: externalDnsConfig.namespace,
		}},
	}
	return append([]client.Object{clusterRole, clusterRoleBinding}, ret...)
}

func listNamespaceRBAC(externalDnsConfig *ExternalDnsConfig) []client.Object {
	clusterRole := &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName + "-list-ns",
			Labels: GetTopLevelLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   externalDnsConfig.resourceName + "-list-ns",
			Labels: GetTopLevelLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     externalDnsConfig.resourceName + "-list-ns",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      externalDnsConfig.serviceAccountName,
			Namespace: externalDnsConfig.namespace,
		}},
	}

	return []client.Object{clusterRole, clusterRoleBinding}
}

func newExternalDNSConfigMap(conf *config.Config, externalDnsConfig *ExternalDnsConfig) (*corev1.ConfigMap, string) {
	jsMap := map[string]interface{}{
		"tenantId":       externalDnsConfig.tenantId,
		"subscriptionId": externalDnsConfig.subscription,
		"resourceGroup":  externalDnsConfig.resourceGroup,
		"cloud":          conf.Cloud,
		"location":       conf.Location,
	}
	jsMap[externalDnsConfig.identityType.externalDNSIdentityConfiguration()] = true

	if externalDnsConfig.identityType == IdentityTypeMSI {
		jsMap["userAssignedIdentityID"] = externalDnsConfig.clientId
	}

	js, err := json.Marshal(&jsMap)
	if err != nil {
		panic(err)
	}
	hash := sha256.Sum256(js)
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName,
			Namespace: externalDnsConfig.namespace,
			Labels:    GetTopLevelLabels(),
		},
		Data: map[string]string{
			"azure.json": string(js),
		},
	}, hex.EncodeToString(hash[:])
}

func newExternalDNSDeployment(conf *config.Config, externalDnsConfig *ExternalDnsConfig, configMapHash string) *appsv1.Deployment {
	domainFilters := []string{}

	for _, zoneId := range externalDnsConfig.dnsZoneResourceIDs {
		parsedZone, err := azure.ParseResourceID(zoneId)
		if err != nil {
			continue
		}
		domainFilters = append(domainFilters, fmt.Sprintf("--domain-filter=%s", parsedZone.ResourceName))
	}

	podLabels := GetTopLevelLabels()
	podLabels["app"] = externalDnsConfig.resourceName
	podLabels["checksum/configmap"] = configMapHash[:checkSumLength]

	if externalDnsConfig.identityType == IdentityTypeWorkloadIdentity {
		podLabels["azure.workload.identity/use"] = "true"
	}

	serviceAccount := externalDnsConfig.serviceAccountName

	txtOwnerArg := "--txt-owner-id=" + conf.ClusterUid
	if externalDnsConfig.isNamespaced {
		txtOwnerArg += "-" + externalDnsConfig.uid
	}

	deploymentArgs := []string{
		"--provider=" + externalDnsConfig.provider.string(),
		"--interval=" + conf.DnsSyncInterval.String(),
		txtOwnerArg,
		"--txt-wildcard-replacement=" + txtWildcardReplacement,
	}

	deploymentArgs = append(deploymentArgs, labelSelectorDeploymentArgs(externalDnsConfig)...)

	resourceTypeArgs := make([]string, 0)
	for resourceType := range externalDnsConfig.resourceTypes {
		resourceTypeArgs = append(resourceTypeArgs, resourceType.generateResourceDeploymentArgs()...)
	}

	sort.Slice(resourceTypeArgs, func(i, j int) bool { return resourceTypeArgs[i] < resourceTypeArgs[j] })
	deploymentArgs = append(deploymentArgs, resourceTypeArgs...)
	deploymentArgs = append(deploymentArgs, domainFilters...)
	deploymentArgs = append(deploymentArgs, namespaceFilterArgs(externalDnsConfig)...)

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalDnsConfig.resourceName,
			Namespace: externalDnsConfig.namespace,
			Labels:    GetTopLevelLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             to.Int32Ptr(replicas),
			RevisionHistoryLimit: util.Int32Ptr(2),
			Selector:             &metav1.LabelSelector{MatchLabels: map[string]string{"app": externalDnsConfig.resourceName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						// https://learn.microsoft.com/en-us/azure/aks/outbound-rules-control-egress#required-outbound-network-rules-and-fqdns-for-aks-clusters
						// helps with firewalls blocking communication to api server
						"kubernetes.azure.com/set-kube-service-host-fqdn": "true",
					},
					Labels: podLabels,
				},
				Spec: *WithPreferSystemNodes(&corev1.PodSpec{
					ServiceAccountName: serviceAccount,
					Containers: []corev1.Container{*withLivenessProbeMatchingReadiness(withTypicalReadinessProbe(7979, &corev1.Container{
						Name:  "controller",
						Image: path.Join(conf.Registry, "/oss/v2/kubernetes/external-dns:v0.17.0"),
						Args:  deploymentArgs,
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "azure-config",
							MountPath: "/etc/kubernetes",
							ReadOnly:  true,
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("250Mi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:               util.ToPtr(false),
							AllowPrivilegeEscalation: util.ToPtr(false),
							ReadOnlyRootFilesystem:   util.ToPtr(true),
							RunAsNonRoot:             util.ToPtr(true),
							RunAsUser:                util.Int64Ptr(65532),
							RunAsGroup:               util.Int64Ptr(65532),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					}))},
					Volumes: []corev1.Volume{{
						Name: "azure-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: externalDnsConfig.resourceName,
								},
							},
						},
					}},
				}),
			},
		},
	}
}

func labelSelectorDeploymentArgs(e *ExternalDnsConfig) []string {
	ret := make([]string, 0)

	if e.gatewayLabelSelector != "" {
		ret = append(ret, "--gateway-label-filter="+e.gatewayLabelSelector)
	}
	if e.routeAndIngressLabelSelector != "" {
		ret = append(ret, "--label-filter="+e.routeAndIngressLabelSelector)
	}

	return ret
}

func namespaceFilterArgs(e *ExternalDnsConfig) []string {
	ret := []string{}
	if e.isNamespaced {
		ret = append(ret, "--namespace="+e.namespace)
		if _, ok := e.resourceTypes[ResourceTypeGateway]; ok {
			ret = append(ret, "--gateway-namespace="+e.namespace)
		}
	}

	return ret
}
