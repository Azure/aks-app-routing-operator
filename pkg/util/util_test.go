package util

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToPtr(t *testing.T) {
	intVar := 2
	require.Equal(t, &intVar, ToPtr(intVar))

	stringVar := "string"
	require.Equal(t, &stringVar, ToPtr(stringVar))
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
