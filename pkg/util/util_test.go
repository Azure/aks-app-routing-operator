package util

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestToPtr(t *testing.T) {
	intVar := 2
	require.Equal(t, &intVar, ToPtr(intVar))

	stringVar := "string"
	require.Equal(t, &stringVar, ToPtr(stringVar))
}

func TestToInt32Ptr(t *testing.T) {
	int32Var := int32(2)
	require.Equal(t, &int32Var, Int32Ptr(int32Var))
}

func TestToInt64Ptr(t *testing.T) {
	int64Var := int64(2)
	require.Equal(t, &int64Var, Int64Ptr(int64Var))
}

func TestUpsert(t *testing.T) {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap",
			Namespace: "default",
		},
		Data: map[string]string{
			"testkey": "testvalue",
		},
	}

	fakeClient := fake.NewClientBuilder().WithObjects(configMap).Build()
	configMap.Data["newkey"] = "newvalue"

	err := Upsert(context.Background(), fakeClient, configMap)
	require.NoError(t, err)

	// ensure both values were merged
	got := &corev1.ConfigMap{}
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(configMap), got)
	require.NoError(t, err)
	require.Equal(t, "testvalue", got.Data["testkey"])
	require.Equal(t, "newvalue", got.Data["newkey"])
}

func TestFindOwnerKind(t *testing.T) {
	cases := []struct {
		name     string
		owners   []metav1.OwnerReference
		kind     string
		expected string
	}{
		{
			name:     "nil owners",
			owners:   nil,
			kind:     "kind",
			expected: "",
		},
		{
			name:     "empty owners",
			owners:   []metav1.OwnerReference{},
			kind:     "kind",
			expected: "",
		},
		{
			name: "non-existent owner",
			owners: []metav1.OwnerReference{{
				Kind: "Kind",
				Name: "Name",
			}},
			kind:     "kind2",
			expected: "",
		},
		{
			name: "existent owner",
			owners: []metav1.OwnerReference{{
				Kind: "Kind",
				Name: "Name",
			}},
			kind:     "Kind",
			expected: "Name",
		},
		{
			name: "existent owner different casing",
			owners: []metav1.OwnerReference{{
				Kind: "kind",
				Name: "name",
			}},
			kind:     "Kind",
			expected: "name",
		},
		{
			name: "existent owner multiple owners",
			owners: []metav1.OwnerReference{
				{
					Kind: "kind",
					Name: "name",
				},
				{
					Kind: "kind2",
					Name: "name2",
				},
			},
			kind:     "Kind2",
			expected: "name2",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FindOwnerKind(c.owners, c.kind)
			require.Equal(t, c.expected, got)
		})
	}
}

func TestJitter(t *testing.T) {
	base := time.Minute

	// out of bounds ratio testing
	require.Equal(t, base, Jitter(base, 0))
	require.Equal(t, base, Jitter(base, 1.2))
	require.Equal(t, base, Jitter(base, 3))
	require.Equal(t, base, Jitter(base, -0.2))
	require.Equal(t, base, Jitter(base, -2))

	// ensure jitter is within bounds
	cases := []float64{0.2, 0.3, 0.75, 0.9, 0.543}
	for _, ratio := range cases {
		t.Run(fmt.Sprintf("ratio-%f", ratio), func(t *testing.T) {
			for i := 0; i < 100; i++ { // run a few times to get the full "range"
				got := Jitter(base, ratio)
				upper := base + time.Duration((float64(base)*ratio)-(float64(base)*(ratio/2)))
				lower := (base + time.Duration(float64(base)*(ratio/2))) * -1
				require.LessOrEqual(t, got, upper)
				require.GreaterOrEqual(t, got, lower)
			}
		})
	}
}

func TestMergeMaps(t *testing.T) {
	cases := []struct {
		name     string
		m1       map[string]string
		m2       map[string]string
		m3       map[string]string
		expected map[string]string
	}{
		{
			name:     "nil maps",
			m1:       nil,
			m2:       nil,
			m3:       nil,
			expected: map[string]string{},
		},
		{
			name:     "empty maps",
			m1:       map[string]string{},
			m2:       map[string]string{},
			m3:       map[string]string{},
			expected: map[string]string{},
		},
		{
			name:     "some nil maps",
			m1:       nil,
			m2:       map[string]string{"one": "two"},
			m3:       nil,
			expected: map[string]string{"one": "two"},
		},
		{
			name:     "equivalent maps",
			m1:       map[string]string{"one": "two"},
			m2:       map[string]string{"one": "two"},
			m3:       map[string]string{"one": "two"},
			expected: map[string]string{"one": "two"},
		},
		{
			name:     "different maps",
			m1:       map[string]string{"one": "two"},
			m2:       map[string]string{"three": "four"},
			m3:       map[string]string{"five": "six"},
			expected: map[string]string{"one": "two", "three": "four", "five": "six"},
		},
		{
			name:     "some overlap",
			m1:       map[string]string{"one": "two"},
			m2:       map[string]string{"one": "two"},
			m3:       map[string]string{"three": "four"},
			expected: map[string]string{"one": "two", "three": "four"},
		},
		{
			name:     "multiple keys",
			m1:       map[string]string{"one": "two", "three": "four"},
			m2:       map[string]string{"three": "four", "five": "six"},
			m3:       map[string]string{"seven": "eight"},
			expected: map[string]string{"one": "two", "three": "four", "five": "six", "seven": "eight"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MergeMaps(c.m1, c.m2, c.m3)
			require.Equal(t, c.expected, got)
		})
	}
}

func TestKeys(t *testing.T) {
	type Case[K comparable, v any] struct {
		name     string
		m        map[K]v
		expected []K
	}

	cases := []Case[string, struct{}]{
		{
			name:     "nil",
			m:        nil,
			expected: []string{},
		},
		{
			name:     "map",
			m:        map[string]struct{}{},
			expected: []string{},
		},
		{
			name:     "one key",
			m:        map[string]struct{}{"one": {}},
			expected: []string{"one"},
		},
		{
			name: "multiple keys",
			m: map[string]struct{}{
				"one":   {},
				"two":   {},
				"three": {},
			},
			expected: []string{"one", "two", "three"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Keys(c.m)
			for _, expected := range c.expected {
				ok := false
				for _, actual := range got {
					if actual == expected {
						ok = true
					}
				}

				require.True(t, ok, "got must contain %s", expected)
			}
		})
	}
}

func TestReverseMap(t *testing.T) {
	cases := []struct {
		name     string
		input    map[string]int
		expected map[int]string
	}{
		{
			name:     "nil",
			input:    nil,
			expected: make(map[int]string),
		},
		{
			name:     "empty",
			input:    map[string]int{},
			expected: make(map[int]string),
		},
		{
			name: "basic map",
			input: map[string]int{
				"one": 1,
				"two": 2,
			},
			expected: map[int]string{
				1: "one",
				2: "two",
			},
		},
		{
			name: "another basic map",
			input: map[string]int{
				"hello": 5,
				"world": 3,
				"foo":   1210,
				"bar":   52,
			},
			expected: map[int]string{
				5:    "hello",
				3:    "world",
				1210: "foo",
				52:   "bar",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ReverseMap(c.input)
			require.Equal(t, c.expected, got)
		})
	}
}

func TestFilterMap(t *testing.T) {
	keyStartsWithAFn := func(k, _ string) bool {
		return strings.HasPrefix(k, "A")
	}

	cases := []struct {
		m        map[string]string
		expected map[string]string
	}{
		{
			m:        map[string]string{"Apple": "one", "Pear": "two", "Apricot": "three"},
			expected: map[string]string{"Apple": "one", "Apricot": "three"},
		},
		{
			m:        map[string]string{"Apple": "three", "Apricot": "four", "Avocado": "five"},
			expected: map[string]string{"Apple": "three", "Apricot": "four", "Avocado": "five"},
		},
		{
			m:        map[string]string{"Orange": "six", "Strawberry": "seven"},
			expected: make(map[string]string),
		},
	}

	for _, c := range cases {
		got := FilterMap(c.m, keyStartsWithAFn)
		require.Equal(t, c.expected, got)
	}
}

func TestNewObject(t *testing.T) {
	t.Run("kubernetes types", func(t *testing.T) {
		tests := []struct {
			name     string
			validate func(t *testing.T)
		}{
			{
				name: "deployment",
				validate: func(t *testing.T) {
					obj := NewObject[*appsv1.Deployment]()
					require.NotNil(t, obj)
					require.IsType(t, &appsv1.Deployment{}, obj)
				},
			},
			{
				name: "pod",
				validate: func(t *testing.T) {
					obj := NewObject[*corev1.Pod]()
					require.NotNil(t, obj)
					require.IsType(t, &corev1.Pod{}, obj)
				},
			},
			{
				name: "ingress",
				validate: func(t *testing.T) {
					obj := NewObject[*networkingv1.Ingress]()
					require.NotNil(t, obj)
					require.IsType(t, &networkingv1.Ingress{}, obj)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, tt.validate)
		}
	})

	t.Run("basic go types", func(t *testing.T) {
		type testStruct struct {
			Name string
			Age  int
		}

		tests := []struct {
			name     string
			validate func(t *testing.T)
		}{
			{
				name: "struct pointer",
				validate: func(t *testing.T) {
					obj := NewObject[*testStruct]()
					require.NotNil(t, obj)
					require.IsType(t, &testStruct{}, obj)
				},
			},
			{
				name: "struct",
				validate: func(t *testing.T) {
					obj := NewObject[testStruct]()
					require.IsType(t, testStruct{}, obj)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, tt.validate)
		}
	})

	t.Run("kubernetes type field initialization", func(t *testing.T) {
		obj := NewObject[*appsv1.Deployment]()
		require.NotNil(t, obj)
		require.Equal(t, "", obj.Name)
		require.Equal(t, "", obj.Namespace)
		require.Nil(t, obj.Spec.Replicas)
	})

	t.Run("client.Object interface", func(t *testing.T) {
		obj := NewObject[*appsv1.Deployment]()
		// Verify it implements client.Object
		_, ok := interface{}(obj).(client.Object)
		require.True(t, ok, "NewObject should return an object that implements client.Object")
	})
}
