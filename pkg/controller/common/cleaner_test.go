package common

import (
	"context"
	"reflect"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	gvk               = gvk2
	namespacedGvr     = gvr2a
	nonNamespacedGvr  = gvr2b
	namespacedList    = gvk.Kind + "List"
	nonNamespacedList = gvk.Kind + "List"

	unstructuredNamespaced   = unstructured.Unstructured{}
	unstucturedNonNamespaced = unstructured.Unstructured{}
	namespace                = "test-namespace"
)

func init() {
	unstructuredNamespaced.SetGroupVersionKind(gvk)
	unstructuredNamespaced.SetName("namespaced-resource-1")
	unstructuredNamespaced.SetNamespace(namespace)
	unstructuredNamespaced.SetLabels(labels1)

	unstucturedNonNamespaced.SetGroupVersionKind(gvk)
	unstucturedNonNamespaced.SetName("non-namespaced-resource-1")
	unstucturedNonNamespaced.SetLabels(labels1)
}

func fakeClientset() *fake.Clientset {
	clientset := fake.NewSimpleClientset()
	clientset.Resources = []*metav1.APIResourceList{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       gvk.Kind,
				APIVersion: gvk.Version,
			},
			GroupVersion: gvk.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{
					Name:       namespacedGvr.Resource,
					Group:      gvk.Group,
					Version:    gvk.Version,
					Kind:       gvk.Kind,
					Namespaced: true,
				},
				{
					Name:       nonNamespacedGvr.Resource,
					Group:      gvk.Group,
					Version:    gvk.Version,
					Kind:       gvk.Kind,
					Namespaced: false,
				},
			},
		},
	}
	return clientset
}

func TestIsNamespaced(t *testing.T) {
	clientset := fakeClientset()

	got, err := isNamespaced(clientset, namespacedGvr)
	require.NoError(t, err, "should not error when fetching valid resource")
	require.True(t, got, "should return true when resource is namespaced")

	got, err = isNamespaced(clientset, nonNamespacedGvr)
	require.NoError(t, err, "should not error when fetching valid resource")
	require.False(t, got, "should return false when resource is not namespaced")

	got, err = isNamespaced(clientset, gvr1)
	require.Error(t, err, "should error when fetching invalid resource")
}

func TestCleanType(t *testing.T) {
	type reactor struct {
		verb, resource string
		fn             k8stesting.ReactionFunc
	}

	tests := []struct {
		name            string
		cleanType       cleanType
		reactors        []reactor
		expectedActions []k8stesting.Action
		expectedErr     error
	}{
		{
			name: "with collection method",
			cleanType: cleanType{
				labels: labels1,
				gvr:    gvr1,
			},
			expectedActions: []k8stesting.Action{DeleteCollectionAction(gvr1, "", labels1)},
			expectedErr:     nil,
		},
		{
			name: "without collection method, namespaced resource",
			cleanType: cleanType{
				labels: labels1,
				gvr:    namespacedGvr,
			},
			reactors: []reactor{
				{
					verb:     "delete-collection",
					resource: namespacedGvr.Resource,
					fn: func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, errors.NewMethodNotSupported(gvr1.GroupResource(), "delete-collection")
					},
				},
				{
					verb:     "list",
					resource: "*",
					fn: func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true,
							&unstructured.UnstructuredList{
								Items: []unstructured.Unstructured{unstructuredNamespaced},
							},
							nil
					},
				},
			},
			expectedActions: []k8stesting.Action{
				DeleteCollectionAction(namespacedGvr, "", labels1),
				ListAction(namespacedGvr, gvk, "", labels1),
				DeleteAction(namespacedGvr, unstructuredNamespaced.GetNamespace(), unstructuredNamespaced.GetName()),
			},
			expectedErr: nil,
		},
		{
			name: "without collection method, nonnamespaced resource",
			cleanType: cleanType{
				labels: labels1,
				gvr:    nonNamespacedGvr,
			},
			reactors: []reactor{
				{
					verb:     "delete-collection",
					resource: nonNamespacedGvr.Resource,
					fn: func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, errors.NewMethodNotSupported(gvr1.GroupResource(), "delete-collection")
					},
				},
				{
					verb:     "list",
					resource: "*",
					fn: func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true,
							&unstructured.UnstructuredList{
								Items: []unstructured.Unstructured{unstucturedNonNamespaced},
							},
							nil
					},
				},
			},
			expectedActions: []k8stesting.Action{
				DeleteCollectionAction(nonNamespacedGvr, "", labels1),
				ListAction(nonNamespacedGvr, gvk, "", labels1),
				DeleteAction(nonNamespacedGvr, "", unstucturedNonNamespaced.GetName()),
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		d := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			namespacedGvr:    namespacedList,
			nonNamespacedGvr: nonNamespacedList,
		})
		for _, reactor := range test.reactors {
			d.PrependReactor(reactor.verb, reactor.resource, reactor.fn)
		}

		c := &cleaner{
			dynamic:   d,
			clientset: fakeClientset(),
			logger:    logr.Discard(),
		}
		require.Equal(t, test.expectedErr, c.CleanType(context.Background(), test.cleanType), "should return expected error")

		for _, action := range test.expectedActions {
			AssertAction(t, d.Actions(), action)
		}
		require.Equal(t, len(test.expectedActions), len(d.Actions()), "should have expected number of actions")
	}
}

func TestCleanerLeaderElection(t *testing.T) {
	var ler manager.LeaderElectionRunnable = &cleaner{}
	require.True(t, ler.NeedLeaderElection(), "should need leader election")
}

func DeleteCollectionAction(gvr schema.GroupVersionResource, namespace string, ls map[string]string) k8stesting.DeleteCollectionAction {
	l := labels.Set(ls)
	return k8stesting.NewDeleteCollectionAction(gvr, namespace, metav1.ListOptions{LabelSelector: l.String()})
}

func DeleteAction(gvr schema.GroupVersionResource, namespace, name string) k8stesting.DeleteAction {
	return k8stesting.NewDeleteAction(gvr, namespace, name)
}

func ListAction(gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace string, ls map[string]string) k8stesting.ListAction {
	l := labels.Set(ls)

	return k8stesting.NewListAction(
		gvr,
		gvk,
		namespace,
		metav1.ListOptions{LabelSelector: l.String()},
	)
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

func TestNewCleaner(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)
	err = NewCleaner(m, controllername.New("test"), RetrieverEmpty())
	require.NoError(t, err)
}

func TestClean(t *testing.T) {
	d := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		namespacedGvr:    namespacedList,
		nonNamespacedGvr: nonNamespacedList,
	})

	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)

	mapper, err := apiutil.NewDynamicRESTMapper(m.GetConfig(), apiutil.WithLazyDiscovery)
	require.NoError(t, err)

	tests := []struct {
		name           string
		c              *cleaner
		expectedErrMsg string
	}{
		{
			name: "nil retriver",
			c: &cleaner{
				name:      controllername.New("test"),
				dynamic:   d,
				clientset: fakeClientset(),
				logger:    logr.Discard(),
			},
			expectedErrMsg: "retriever is nil",
		},
		{
			name: "runs clean without err",
			c: &cleaner{
				name:      controllername.New("test"),
				dynamic:   d,
				clientset: fakeClientset(),
				logger:    logr.Discard(),
				mapper:    mapper,
				retriever: RetrieverEmpty(),
			},
			expectedErrMsg: "",
		},
	}

	for _, test := range tests {
		err := test.c.Clean(context.Background())
		if err != nil {
			require.Equal(t, test.expectedErrMsg, err.Error(), "should return expected error msg")
		}
	}
}

func TestCleanerStart(t *testing.T) {
	d := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		namespacedGvr:    namespacedList,
		nonNamespacedGvr: nonNamespacedList,
	})

	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)

	mapper, err := apiutil.NewDynamicRESTMapper(m.GetConfig(), apiutil.WithLazyDiscovery)
	require.NoError(t, err)

	tests := []struct {
		name           string
		c              *cleaner
		expectedErrMsg string
	}{
		{
			name: "failing to start",
			c: &cleaner{
				name:       controllername.New("test"),
				dynamic:    d,
				clientset:  fakeClientset(),
				logger:     logr.Discard(),
				maxRetries: 1,
			},
			expectedErrMsg: "retriever is nil",
		},
		{
			name: "starts cleaner",
			c: &cleaner{
				name:       controllername.New("test"),
				dynamic:    d,
				clientset:  fakeClientset(),
				logger:     logr.Discard(),
				mapper:     mapper,
				retriever:  RetrieverEmpty(),
				maxRetries: 1,
			},
			expectedErrMsg: "",
		},
	}
	for _, test := range tests {
		err := test.c.Start(context.Background())
		if err != nil {
			require.Equal(t, test.expectedErrMsg, err.Error(), "should return expected error msg")
		}
	}
}
