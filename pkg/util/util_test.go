package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
