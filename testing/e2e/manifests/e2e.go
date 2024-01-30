package manifests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	e2eNamespace = "kube-system"
)

func E2e(image, loadableProvisionedJson string) []client.Object {
	ret := []client.Object{
		&corev1.ServiceAccount{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ServiceAccount",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-e2e",
				Namespace: e2eNamespace,
			},
		},
		&rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRoleBinding",
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-routing-e2e",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      "app-routing-e2e",
					Namespace: e2eNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     "cluster-admin",
				APIGroup: "",
			},
		},
		&corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "infrastructure",
				Namespace: e2eNamespace,
			},
			Data: map[string]string{
				"infra-config.json": loadableProvisionedJson,
			},
		},
		&batchv1.Job{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Job",
				APIVersion: batchv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-routing-operator-e2e",
				Namespace: e2eNamespace,
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: to.Ptr(int32(0)), // this is number of retries, we only want to try once
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: "app-routing-e2e",
						RestartPolicy:      corev1.RestartPolicyNever,
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
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "infrastructure"},
									},
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
