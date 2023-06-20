package common

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func groupVersionResource(client client.Client, obj client.Object) schema.GroupVersionResource {
	gvk := obj.GetObjectKind().GroupVersionKind()
	gk := schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}

	mapping, _ := client.RESTMapper().RESTMapping(gk, gvk.Version)
	return mapping.Resource
}

func TestCleanerEmptyRetriever(t *testing.T) {
	d := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	c := &cleaner{
		name:         "test-name",
		dynamic:      d,
		logger:       logr.Discard(),
		gvrRetriever: nil,
		labels:       nil,
	}
	require.NoError(t, c.Start(context.Background()))
	require.NotNil(t, c.Clean(context.Background()))
}

func TestCleanerEmptyLabels(t *testing.T) {
	d := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	c := &cleaner{
		name:    "test-name",
		dynamic: d,
		logger:  logr.Discard(),
		gvrRetriever: func(client client.Client) ([]schema.GroupVersionResource, error) {
			return []schema.GroupVersionResource{
				{
					Group:    "apps",
					Version:  "v1",
					Resource: "namespace",
				}}, nil

		}}
	require.NoError(t, c.Clean(context.Background()))
}

func TestCleanerIntegration(t *testing.T) {
	labels := labels.Set(map[string]string{
		"testing": "testing",
		"another": "label",
	})

	type reactors struct {
		verb, resource string
		fn             k8stesting.ReactionFunc
	}

	tests := []struct {
		name     string
		objs     []testingObj
		reactors []reactors
		actions  []k8stesting.Action
		schemeFn func() *runtime.Scheme
	}{
		{
			name: "single resource with no collection",
			objs: []testingObj{
				TestingObj("core", "v1", "Service", "object", "services", labels),
			},
			reactors: []reactors{
				{
					verb:     "delete-collection",
					resource: "services",
					fn: func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, errors.NewMethodNotSupported(schema.GroupResource{}, "action")
					},
				},
			},
			actions: []k8stesting.Action{
				k8stesting.NewDeleteCollectionAction(schema.GroupVersionResource{
					Group:    "core",
					Version:  "v1",
					Resource: "services",
				}, "", metav1.ListOptions{LabelSelector: labels.String()}),
				k8stesting.NewListAction(
					schema.GroupVersionResource{
						Group:    "core",
						Version:  "v1",
						Resource: "services",
					}, schema.GroupVersionKind{
						Group:   "core",
						Version: "v1",
						Kind:    "Service",
					},
					"",
					metav1.ListOptions{LabelSelector: labels.String()}),
			},
			schemeFn: func() *runtime.Scheme {
				return runtime.NewScheme()
			},
		},
		{
			name: "multiple resources with collections",
			objs: []testingObj{
				TestingObj("core", "v1", "Pod", "object1", "pods", labels),
				TestingObj("core", "v1", "Deployment", "object2", "deployments", labels),
			},
			reactors: []reactors{},
			actions: []k8stesting.Action{
				k8stesting.NewDeleteCollectionAction(schema.GroupVersionResource{
					Group:    "core",
					Version:  "v1",
					Resource: "pods",
				}, "", metav1.ListOptions{LabelSelector: labels.String()}),
				k8stesting.NewDeleteCollectionAction(schema.GroupVersionResource{
					Group:    "core",
					Version:  "v1",
					Resource: "pods",
				}, "", metav1.ListOptions{LabelSelector: labels.String()}),
			},
			schemeFn: func() *runtime.Scheme {
				return runtime.NewScheme()
			},
		},
	}

	for _, test := range tests {
		var objs []runtime.Object
		var gvrs []schema.GroupVersionResource
		for _, obj := range test.objs {
			objs = append(objs, obj.unstructured)
			gvrs = append(gvrs, obj.gvr)
		}
		d := dynamicfake.NewSimpleDynamicClient(test.schemeFn(), objs...)
		for _, reactor := range test.reactors {
			d.AddReactor(reactor.verb, reactor.resource, reactor.fn)
		}

		c := &cleaner{
			name:    "test-name",
			dynamic: d,
			logger:  logr.Discard(),
			gvrRetriever: func(_ client.Client) ([]schema.GroupVersionResource, error) {
				return gvrs, nil
			},
			labels: labels,
		}
		require.NoError(t, c.Start(context.Background()))

		// we can't check if objects are actually deleted because delete collection isn't implemented by fakestore
		// https://github.com/kubernetes/client-go/issues/609
		// https://github.com/kubernetes/kubernetes/issues/105357
		// we have to compare to actions instead
		for _, action := range test.actions {
			AssertAction(t, d.Actions(), action)
		}
	}

}

type testingObj struct {
	unstructured *unstructured.Unstructured
	gvr          schema.GroupVersionResource
}

func TestingObj(group, version, kind, name, resource string, labels map[string]string) testingObj {
	unstructured := &unstructured.Unstructured{}
	unstructured.SetAPIVersion(fmt.Sprintf("%s/%s", group, version))
	unstructured.SetKind(kind)
	unstructured.SetName(name)
	unstructured.SetLabels(labels)

	return testingObj{
		unstructured: unstructured,
		gvr: schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: resource,
		},
	}
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
