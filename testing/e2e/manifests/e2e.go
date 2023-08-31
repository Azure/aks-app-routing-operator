package manifests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func E2e(image, loadableProvisionedJson string) []client.Object {
	ret := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "infrastructure",
			},
			Data: map[string]string{
				"infra-config.json": string(loadableProvisionedJson),
			},
		},
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
								Args:  []string{"test", "--infra-file", "/infrastructure/infra-config.json"},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "infra-volume",
										MountPath: "/infrastructure/infra-config.json",
										SubPath:   "infra-config.json",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "infra-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "infrastructure"}},
								},
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
