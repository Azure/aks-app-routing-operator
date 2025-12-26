package manifests

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ManagedByKey = "app.kubernetes.io/managed-by"
	// ManagedByVal is the value for the ManagedByKey label on all resources directly managed by our e2e tester
	ManagedByVal = "app-routing-operator-e2e"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	// add any types used in this package
	batchv1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	metav1.AddMetaToScheme(scheme)
	appsv1.AddToScheme(scheme)
	policyv1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)
	gatewayv1.Install(scheme)
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

func setGroupKindVersion(obj client.Object) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return
	}

	if len(gvks) == 0 {
		return
	}

	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
}

// newGoDeployment creates a new basic Go deployment with a single main.go file from contents
func newGoDeployment(contents, namespace, name string) *appsv1.Deployment {
	command := []string{
		"/bin/sh",
		"-c",
		"mkdir source && cd source && go mod init source && echo '" + contents + "' > main.go && go mod tidy && go run main.go",
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": name},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "container",
						Image:   "mcr.microsoft.com/oss/go/microsoft/golang:1.23",
						Command: command,
					}},
				},
			},
		},
	}
}

// UncollisionedNs returns a namespace with a guaranteed unique name after creating the namespace
func UncollisionedNs() *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "app-routing-e2e-",
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
	}
}
