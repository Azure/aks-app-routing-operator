package common

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestIsNamespaced(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.Resources = []*metav1.APIResourceList{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       gvk2.Kind,
				APIVersion: gvk2.Version,
			},
			GroupVersion: gvk2.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Name:       gvr2a.Resource,
					Group:      gvk2.Group,
					Version:    gvk2.Version,
					Kind:       gvk2.Kind,
					Namespaced: true,
				},
				{
					Name:       gvr2b.Resource,
					Group:      gvk2.Group,
					Version:    gvk2.Version,
					Kind:       gvk2.Kind,
					Namespaced: false,
				},
			},
		},
	}

	got, err := isNamespaced(clientset, gvr2a)
	require.NoError(t, err, "should not error when fetching valid resource")
	require.True(t, got, "should return true when resource is namespaced")

	got, err = isNamespaced(clientset, gvr2b)
	require.NoError(t, err, "should not error when fetching valid resource")
	require.False(t, got, "should return false when resource is not namespaced")

	got, err = isNamespaced(clientset, gvr1)
	require.Error(t, err, "should error when fetching invalid resource")
}

func AssertAction(t *testing.T, got []k8stesting.Action, expected k8stesting.Action) {
	found := false
	for _, g := range got {
		if reflect.DeepEqual(expected, g) {
			found = true
			break
		}
	}

	require.True(t, found, "expected to find action", expected)
}
