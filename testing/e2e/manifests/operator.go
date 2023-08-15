package manifests

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	operatorDeploymentLabels = map[string]string{
		"app": "app-routing-operator",
	}
)

type OperatorConfig struct {
	Msi        string
	TenantId   string
	Location   string
	DnsZoneIds []string
	ClusterUid string
}

// args returns the arguments to pass to the operator
func (o *OperatorConfig) args() []string {
	ret := []string{
		"--msi", o.Msi,
		"--tenant-id", o.TenantId,
		"--location", o.Location,
		"--cluster-uid", o.ClusterUid,
	}

	if len(o.DnsZoneIds) > 0 {
		ret = append(ret, "--dns-zone-ids", strings.Join(o.DnsZoneIds, ","))
	}

	return ret
}

func Operator(image string, cfg *OperatorConfig) []client.Object {
	return []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator",
				Namespace: "kube-system",
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
								Image: image,
								Command: []string{
									"/app-routing-operator",
								},
								Args: cfg.args(),
							},
						},
					},
				},
			},
		},
	}
}
