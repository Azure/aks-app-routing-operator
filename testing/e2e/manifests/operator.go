package manifests

import (
	"math"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	operatorDeploymentLabels = map[string]string{
		"app": "app-routing-operator",
	}
)

// OperatorVersion is an enum for the different versions of the operator
type OperatorVersion uint

const (
	OperatorVersion0_0_3 OperatorVersion = iota // use iota to number with earlier versions being lower numbers

	// OperatorVersionLatest represents the latest version of the operator which is essentially whatever code changes this test is running against
	OperatorVersionLatest = math.MaxUint // this must always be the last/largest value in the enum because we order by value
)

// DnsZoneCount is enum for the number of dns zones but shouldn't be used directly. Use the exported fields of this type instead.
type DnsZoneCount uint

const (
	// DnsZoneCountNone represents no dns zones
	DnsZoneCountNone DnsZoneCount = iota
	// DnsZoneCountOne represents one dns zone
	DnsZoneCountOne
	// DnsZoneCountMultiple represents multiple dns zones
	DnsZoneCountMultiple
)

type DnsZones struct {
	Public  DnsZoneCount
	Private DnsZoneCount
}

type OperatorConfig struct {
	Version    OperatorVersion
	Msi        string
	TenantId   string
	Location   string
	Zones      DnsZones
	DisableOsm bool
}

func (o *OperatorConfig) image(latestImage string) string {
	switch o.Version {
	case OperatorVersion0_0_3:
		return "mcr.microsoft.com/aks/app-routing-operator:0.0.3"
	default:
		return latestImage
	}
}

// args returns the arguments to pass to the operator
func (o *OperatorConfig) args(publicZones, privateZones []string) []string {
	// there's no difference in cli flags between version 0.0.3 and the current one
	// so we can use the same args for both. this won't always be true. logic will be
	// added here to modify the args based on the version when needed

	if len(publicZones) < 2 || len(privateZones) < 2 {
		panic("not enough zones provided")
	}

	ret := []string{
		"--msi", o.Msi,
		"--tenant-id", o.TenantId,
		"--location", o.Location,
		"--namespace", "app-routing-system",
	}

	var zones []string
	switch o.Zones.Public {
	case DnsZoneCountMultiple:
		zones = append(zones, publicZones...)
	case DnsZoneCountOne:
		zones = append(zones, publicZones[0])
	}
	switch o.Zones.Private {
	case DnsZoneCountMultiple:
		zones = append(zones, privateZones...)
	case DnsZoneCountOne:
		zones = append(zones, privateZones[0])
	}
	if len(zones) > 0 {
		ret = append(ret, "--dns-zone-ids", strings.Join(zones, ","))
	}

	if o.DisableOsm {
		ret = append(ret, "--disable-osm")
	}

	return ret
}

func Operator(latestImage string, publicZones, privateZones []string, cfg *OperatorConfig) []client.Object {
	ret := []client.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-routing-system",
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "app-routing-system",
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "app-routing-system",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "app-routing-operator",
					Namespace: "app-routing-system",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     "cluster-admin",
				APIGroup: "",
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "app-routing-system", // we use app-routing-system for now, so we can take advantage of ownership refs and garbage collection
				Labels: map[string]string{
					ManagedByKey: ManagedByVal,
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: to.Ptr(int32(2)),
				Selector: &metav1.LabelSelector{
					MatchLabels: operatorDeploymentLabels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: operatorDeploymentLabels,
					},
					Spec: corev1.PodSpec{
						ServiceAccountName: "app-routing-operator",
						Containers: []corev1.Container{
							{
								Name:  "operator",
								Image: cfg.image(latestImage),
								Command: []string{
									"/app-routing-operator",
								},
								Args: cfg.args(publicZones, privateZones),
							},
						},
					},
				},
			},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "app-routing-system",
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: to.Ptr(intstr.FromInt(1)),
				Selector: &metav1.LabelSelector{
					MatchLabels: operatorDeploymentLabels,
				},
			},
		},
	}

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}
