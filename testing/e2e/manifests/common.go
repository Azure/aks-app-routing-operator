package manifests

import (
	"fmt"

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
	// any new schemes used should be added here so
	// we can marshal them
	batchv1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	metav1.AddMetaToScheme(scheme)
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
