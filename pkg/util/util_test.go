package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
