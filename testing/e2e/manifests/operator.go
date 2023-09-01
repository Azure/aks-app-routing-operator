package manifests

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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
type OperatorVersion int

const (
	OperatorVersion0_0_3 OperatorVersion = iota // use iota to number with earlier versions being lower numbers
	OperatorVersionLatest
)

type DnsZoneCount int

const (
	DnsZoneCountNone DnsZoneCount = iota
	DnsZoneCountOne
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
	ClusterUid string
	disableOsm bool
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
		"--cluster-uid", o.ClusterUid,
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

	if o.disableOsm {
		ret = append(ret, "--disable-osm")
	}

	return ret
}

func Operator(latestImage string, publicZones, privateZones []string, cfg *OperatorConfig) []client.Object {
	ret := []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "app-routing-system", // we use app-routing-system for now so we can take advantage of ownership refs and garbage collection
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
				Name: "app-routing-operator",
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