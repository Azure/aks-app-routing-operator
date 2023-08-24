package manifests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func E2e(image, infraName string) []client.Object {
	ret := []client.Object{
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-routing-operator-e2e",
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: to.Ptr(int32(1)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:  "app-routing-operator-e2e",
								Image: image,
								Args:  []string{"test", "--infra-name", infraName},
							},
						},
					},
				},
			},
		},
	}

	// set the group kind and version for each object
	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}
