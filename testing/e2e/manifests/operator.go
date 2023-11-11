package manifests

import (
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"math"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	operatorNs        = "kube-system"
	managedResourceNs = "app-routing-system"
)

var (
	operatorDeploymentLabels = map[string]string{
		"app": "app-routing-operator",
	}

	// AllOperatorVersions is a list of all the operator versions
	AllOperatorVersions = []OperatorVersion{OperatorVersion0_0_3, OperatorVersionLatest}

	// AllDnsZoneCounts is a list of all the dns zone counts
	AllDnsZoneCounts     = []DnsZoneCount{DnsZoneCountNone, DnsZoneCountOne, DnsZoneCountMultiple}
	NonZeroDnsZoneCounts = []DnsZoneCount{DnsZoneCountOne, DnsZoneCountMultiple}

	SingleStackIPFamilyPolicy = corev1.IPFamilyPolicySingleStack
)

// OperatorVersion is an enum for the different versions of the operator
type OperatorVersion uint

const (
	OperatorVersion0_0_3 OperatorVersion = iota // use iota to number with earlier versions being lower numbers

	// OperatorVersionLatest represents the latest version of the operator which is essentially whatever code changes this test is running against
	OperatorVersionLatest = math.MaxUint // this must always be the last/largest value in the enum because we order by value
)

func (o OperatorVersion) String() string {
	switch o {
	case OperatorVersion0_0_3:
		return "0.0.3"
	case OperatorVersionLatest:
		return "latest"
	default:
		return "unknown"
	}
}

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

func (d DnsZoneCount) String() string {
	switch d {
	case DnsZoneCountNone:
		return "none"
	case DnsZoneCountOne:
		return "one"
	case DnsZoneCountMultiple:
		return "multiple"
	default:
		return "unknown"
	}
}

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
		return "mcr.microsoft.com/aks/aks-app-routing-operator:0.0.3"
	case OperatorVersionLatest:
		return latestImage
	default:
		panic("unknown operator version")
	}
}

// args returns the arguments to pass to the operator
func (o *OperatorConfig) args(publicZones, privateZones []string) []string {
	if len(publicZones) < 2 || len(privateZones) < 2 {
		panic("not enough zones provided")
	}

	ret := []string{
		"--msi", o.Msi,
		"--tenant-id", o.TenantId,
		"--location", o.Location,
		"--namespace", managedResourceNs,
		"--cluster-uid", "test-cluster-uid",
	}

	if o.Version == OperatorVersionLatest {
		ret = append(ret, "--dns-sync-interval", (time.Second * 15).String())
		ret = append(ret, "--operator-namespace", operatorNs)
		ret = append(ret, "--operator-webhook-service", "app-routing-operator-webhook")
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

func Operator(latestImage string, publicZones, privateZones []string, cfg *OperatorConfig, cleanDeploy bool) []client.Object {
	var ret []client.Object

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: managedResourceNs,
		},
	}

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
		},
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "app-routing-operator",
				Namespace: operatorNs,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "",
		},
	}

	baseDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
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
							Args:  cfg.args(publicZones, privateZones),
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath: "/tmp/k8s-webhook-server/serving-certs",
									Name:      "cert",
									ReadOnly:  true,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.IntOrString{
											IntVal: 8080,
											Type:   intstr.Int,
										},
										HTTPHeaders: nil,
									},
								},
								PeriodSeconds: 5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.IntOrString{
											IntVal: 8080,
											Type:   intstr.Int,
										},
										HTTPHeaders: nil,
									},
								},
								PeriodSeconds: 5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "app-routing-webhook-secret",
									Optional:    util.BoolPtr(true),
									DefaultMode: util.Int32Ptr(420),
								},
							},
						},
					},
				},
			},
		},
	}

	podDisrutptionBudget := &policyv1.PodDisruptionBudget{
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
	}

	webhookService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator-webhook",
			Namespace: operatorNs,
		},
		Spec: corev1.ServiceSpec{
			IPFamilies: []corev1.IPFamily{
				"IPv4",
			},
			IPFamilyPolicy: &SingleStackIPFamilyPolicy,
			Ports: []corev1.ServicePort{
				{
					Name:     "client",
					Port:     9443,
					Protocol: corev1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 9443,
					},
				},
			},
			Selector: map[string]string{
				"app": "app-routing-operator",
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeClusterIP,
		},
	}

	ret = append(ret, []client.Object{
		namespace,
		serviceAccount,
		clusterRoleBinding,
		podDisrutptionBudget,
	}...)

	// edit and select relevant manifest config by version
	switch cfg.Version {
	case OperatorVersion0_0_3:
		baseDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
		baseDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
		baseDeployment.Spec.Template.Spec.Containers[0].StartupProbe = nil
		baseDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = nil
		baseDeployment.Spec.Template.Spec.Volumes = nil

		ret = append(ret, baseDeployment)
	case OperatorVersionLatest:
		ret = append(ret, webhookService, baseDeployment)

		if cleanDeploy {
			ret = append(ret, NewNginxIngressController("default", "webapprouting.kubernetes.azure.com"))
		}
	}

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}
