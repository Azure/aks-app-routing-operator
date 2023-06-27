package common

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	labels1 = map[string]string{
		"key":   "val",
		"other": "val2",
	}
	labels2 = map[string]string{
		"key": "val",
	}
	gvr1 = schema.GroupVersionResource{
		Group:    "group",
		Version:  "v1",
		Resource: "resources",
	}
	gvr2a = schema.GroupVersionResource{
		Group:    "group",
		Version:  "v2",
		Resource: "resources2",
	}
	gvr2b = schema.GroupVersionResource{
		Group:    "group",
		Version:  "v2",
		Resource: "resources3",
	}
	gvk1 = schema.GroupVersionKind{
		Group:   gvr1.Group,
		Version: gvr1.Version,
		Kind:    "Resource",
	}
	gvk2 = schema.GroupVersionKind{
		Group:   gvr2a.Group,
		Version: gvr2a.Version,
		Kind:    "Resource2",
	}
)

type testMapper struct {
	meta.RESTMapper
}

func (t testMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	switch gk {
	case gvk1.GroupKind():
		return []*meta.RESTMapping{{
			Resource:         gvr1,
			GroupVersionKind: gvk1,
		}}, nil
	case gvk2.GroupKind():
		return []*meta.RESTMapping{
			{
				Resource:         gvr2a,
				GroupVersionKind: gvk2,
			},
			{
				Resource:         gvr2b,
				GroupVersionKind: gvk2,
			}}, nil
	default:
		return nil, fmt.Errorf("unknown kind %s", gk.Kind)
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		name     string
		c1, c2   cleanType
		cs       CompareStrategy
		expected bool
	}{
		{
			name:     "empty structs",
			c1:       cleanType{},
			c2:       cleanType{},
			cs:       Everything,
			expected: true,
		},
		{
			name:     "empty structs ignore labels",
			c1:       cleanType{},
			c2:       cleanType{},
			cs:       IgnoreLabels,
			expected: true,
		},
		{
			name: "different labels",
			c1: cleanType{
				labels: labels1,
				gvr:    gvr1,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr1,
			},
			cs:       Everything,
			expected: false,
		},
		{
			name: "different labels ignore labels",
			c1: cleanType{
				labels: labels1,
				gvr:    gvr1,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr1,
			},
			cs:       IgnoreLabels,
			expected: true,
		},
		{
			name: "same gvr same label",
			c1: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			cs:       Everything,
			expected: true,
		},
		{
			name: "same gvr same label ignore labels",
			c1: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			cs:       IgnoreLabels,
			expected: true,
		},
		{
			name: "different gvr same label",
			c1: cleanType{
				labels: labels2,
				gvr:    gvr1,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			cs:       Everything,
			expected: false,
		},
		{
			name: "different gvr same label ignore label",
			c1: cleanType{
				labels: labels2,
				gvr:    gvr1,
			},
			c2: cleanType{
				labels: labels2,
				gvr:    gvr2a,
			},
			cs:       IgnoreLabels,
			expected: false,
		},
	}

	for _, test := range tests {
		got := equal(test.c1, test.c2, test.cs)
		if got != test.expected {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}

}

func TestAddLabels(t *testing.T) {
	tests := []struct {
		name     string
		gvrs     []schema.GroupVersionResource
		labels   map[string]string
		expected []cleanType
	}{
		{
			name:     "empty gvrs",
			gvrs:     nil,
			labels:   nil,
			expected: []cleanType(nil),
		},
		{
			name: "one gvr",
			gvrs: []schema.GroupVersionResource{
				gvr1,
			},
			labels: labels1,
			expected: []cleanType{
				{
					labels: labels1,
					gvr:    gvr1,
				},
			},
		},
		{
			name: "multiple gvrs",
			gvrs: []schema.GroupVersionResource{
				gvr1,
				gvr2a,
			},
			labels: labels2,
			expected: []cleanType{
				{
					labels: labels2,
					gvr:    gvr1,
				},
				{
					labels: labels2,
					gvr:    gvr2a,
				},
			},
		},
	}

	for _, test := range tests {
		got := addLabels(test.gvrs, test.labels)
		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func TestGvrsFromGk(t *testing.T) {
	tests := []struct {
		name        string
		gk          schema.GroupKind
		expected    []schema.GroupVersionResource
		expectedErr bool
	}{
		{
			name:        "empty gk",
			gk:          schema.GroupKind{},
			expected:    []schema.GroupVersionResource(nil),
			expectedErr: true,
		},
		{
			name:        "gk with no matches",
			gk:          schema.GroupKind{Group: "foo", Kind: "bar"},
			expected:    []schema.GroupVersionResource(nil),
			expectedErr: true,
		},
		{
			name:        "gk with matches single gvr",
			gk:          gvk1.GroupKind(),
			expected:    []schema.GroupVersionResource{gvr1},
			expectedErr: false,
		},
		{
			name:        "gk with matches multiple gvrs",
			gk:          gvk2.GroupKind(),
			expected:    []schema.GroupVersionResource{gvr2a, gvr2b},
			expectedErr: false,
		},
	}

	for _, test := range tests {
		got, err := gvrsFromGk(testMapper{}, test.gk)

		if (test.expectedErr && err == nil) ||
			(!test.expectedErr && err != nil) {
			t.Error(
				"For", test.name,
				"got", err,
				"error should exist", test.expectedErr,
			)
		}

		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func TestRetrieverFromObjs(t *testing.T) {
	tests := []struct {
		name        string
		objs        []client.Object
		labels      map[string]string
		expected    []cleanType
		expectedErr error
	}{
		{
			name:        "empty objs",
			objs:        nil,
			labels:      nil,
			expected:    []cleanType(nil),
			expectedErr: nil,
		},
		{
			name: "one obj one gvr one label",
			objs: []client.Object{
				obj(gvk1, labels1),
			},
			labels:      labels1,
			expected:    []cleanType{{gvr: gvr1, labels: labels1}},
			expectedErr: nil,
		},
		{
			name:   "one obj multiple gvr one label",
			objs:   []client.Object{obj(gvk2, labels1)},
			labels: labels1,
			expected: []cleanType{
				{gvr: gvr2a, labels: labels1},
				{gvr: gvr2b, labels: labels1},
			},
			expectedErr: nil,
		},
		{
			name:   "multiple objects",
			objs:   []client.Object{obj(gvk1, labels1), obj(gvk2, labels2)},
			labels: labels2,
			expected: []cleanType{
				{gvr: gvr1, labels: labels2},
				{gvr: gvr2a, labels: labels2},
				{gvr: gvr2b, labels: labels2},
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		fn := RetrieverFromObjs(test.objs, test.labels)
		got, err := fn(testMapper{})
		if err != test.expectedErr {
			t.Error(
				"For", test.name,
				"got", err,
				"expected", test.expectedErr,
			)
		}

		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func TestRetrieverFromGk(t *testing.T) {
	tests := []struct {
		name        string
		gks         []schema.GroupKind
		labels      map[string]string
		expected    []cleanType
		expectedErr error
	}{
		{
			name:        "empty gk",
			gks:         nil,
			labels:      nil,
			expected:    nil,
			expectedErr: nil,
		},
		{
			name:        "one gk one gvr one label",
			gks:         []schema.GroupKind{gvk1.GroupKind()},
			labels:      labels1,
			expected:    []cleanType{{gvr: gvr1, labels: labels1}},
			expectedErr: nil,
		},
		{
			name:   "one gk multiple gvrs one label",
			gks:    []schema.GroupKind{gvk2.GroupKind()},
			labels: labels1,
			expected: []cleanType{
				{gvr: gvr2a, labels: labels1},
				{gvr: gvr2b, labels: labels1},
			},
			expectedErr: nil,
		},
		{
			name:   "multiple gks",
			gks:    []schema.GroupKind{gvk1.GroupKind(), gvk2.GroupKind()},
			labels: labels2,
			expected: []cleanType{
				{gvr: gvr1, labels: labels2},
				{gvr: gvr2a, labels: labels2},
				{gvr: gvr2b, labels: labels2},
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		fn := RetrieverFromGk(test.labels, test.gks...)
		got, err := fn(testMapper{})
		if err != test.expectedErr {
			t.Error(
				"For", test.name,
				"got", err,
				"expected", test.expectedErr,
			)
		}

		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func TestRetrieverEmpty(t *testing.T) {
	got, err := RetrieverEmpty()(testMapper{})
	require.NoError(t, err)
	require.Equal(t, []cleanType{}, got)
}

func TestCleanTypeRetrieverAdd(t *testing.T) {
	tests := []struct {
		name        string
		reciever    CleanTypeRetriever
		addition    CleanTypeRetriever
		expected    []cleanType
		expectedErr error
	}{
		{
			name:     "empty receiver, one add",
			reciever: RetrieverEmpty(),
			addition: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			expected:    addLabels([]schema.GroupVersionResource{gvr1}, labels1),
			expectedErr: nil,
		},
		{
			name: "one receiver, zero add",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			addition:    RetrieverEmpty(),
			expected:    addLabels([]schema.GroupVersionResource{gvr1}, labels1),
			expectedErr: nil,
		},
		{
			name: "one receiver, multiple add",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			addition: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr2a, gvr2b}, labels2), nil
			},
			expected:    []cleanType{{gvr: gvr1, labels: labels1}, {gvr: gvr2a, labels: labels2}, {gvr: gvr2b, labels: labels2}},
			expectedErr: nil,
		},
		{
			name: "multiple receiver, one add",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr2a, gvr2b}, labels1), nil
			},
			addition: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels2), nil
			},
			expected:    []cleanType{{gvr: gvr2a, labels: labels1}, {gvr: gvr2b, labels: labels1}, {gvr: gvr1, labels: labels2}},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		got, err := test.reciever.Add(test.addition)(testMapper{})
		if err != test.expectedErr {
			t.Error(
				"For", test.name,
				"got", err,
				"expected", test.expectedErr,
			)
		}

		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func TestCleanTypeRetrieverRemove(t *testing.T) {
	tests := []struct {
		name        string
		reciever    CleanTypeRetriever
		deletion    CleanTypeRetriever
		opt         RemoveOpt
		expected    []cleanType
		expectedErr error
	}{
		{
			name:     "empty receiver, one delete",
			reciever: RetrieverEmpty(),
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			expected:    []cleanType(nil),
			expectedErr: nil,
		},
		{
			name: "one receiver, zero delete",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			deletion:    RetrieverEmpty(),
			expected:    addLabels([]schema.GroupVersionResource{gvr1}, labels1),
			expectedErr: nil,
		},
		{
			name: "one receiver, one delete, overlap with default compare",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			expected:    []cleanType(nil),
			expectedErr: nil,
		},
		{
			name: "one receiver, one delete, overlap with ignore labels",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			opt:         RemoveOpt{CompareStrat: IgnoreLabels},
			expected:    []cleanType(nil),
			expectedErr: nil,
		},
		{
			name: "one receiver, one delete, overlap with ignore labels, different labels",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels1), nil
			},
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1}, labels2), nil
			},
			opt:         RemoveOpt{CompareStrat: IgnoreLabels},
			expected:    []cleanType(nil),
			expectedErr: nil,
		},
		{
			name: "multiple receiver, multiple delete, overlap labels",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1, gvr2a, gvr2b}, labels1), nil
			},
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr2a, gvr2b}, labels1), nil
			},
			expected:    []cleanType{{gvr: gvr1, labels: labels1}},
			expectedErr: nil,
		},
		{
			name: "multiple receiver, multiple delete, different labels",
			reciever: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr1, gvr2a, gvr2b}, labels1), nil
			},
			deletion: func(mapper meta.RESTMapper) ([]cleanType, error) {
				return addLabels([]schema.GroupVersionResource{gvr2a, gvr2b}, labels2), nil
			},
			expected:    []cleanType{{gvr: gvr1, labels: labels1}, {gvr: gvr2a, labels: labels1}, {gvr: gvr2b, labels: labels1}},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		got, err := test.reciever.Remove(test.deletion, test.opt)(testMapper{})
		if err != test.expectedErr {
			t.Error(
				"For", test.name,
				"got", err,
				"expected", test.expectedErr,
			)
		}

		if !reflect.DeepEqual(got, test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}
}

func obj(gvk schema.GroupVersionKind, labels map[string]string) client.Object {
	o := &unstructured.Unstructured{}
	o.SetLabels(labels)
	o.SetGroupVersionKind(gvk)
	return o
}
