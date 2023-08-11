package manifests

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	batchv1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	metav1.AddMetaToScheme(scheme)
}

func E2e(image, infraName string) []client.Object {
	return []client.Object{
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-routing-operator-e2e",
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: to.Ptr(int32(2)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:    "app-routing-operator-e2e",
								Image:   image,
								Command: []string{"test", "--infra-name", infraName},
							},
						},
					},
				},
			},
		},
	}
}

// MarshalJson converts an object to json
func MarshalJson(obj client.Object) ([]byte, error) {
	codec := serializer.NewCodecFactory(scheme).LegacyCodec(scheme.PreferredVersionAllGroups()...)
	yml, err := runtime.Encode(codec, obj)
	if err != nil {
		return nil, fmt.Errorf("encoding object: %w", err)
	}

	return yml, nil
}
