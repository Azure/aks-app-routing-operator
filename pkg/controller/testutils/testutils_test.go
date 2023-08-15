package testutils

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIsPrometheusBestPracticeName(t *testing.T) {
	notSnakeCase := "obviouslyNotSnakeCase"
	simpleSnakeCase := "snake_case"
	complexSnakeCase := "complex_snake_case"
	leadingSlash := "_leading_slash"
	trailingSlash := "trailing_slash_"

	require.False(t, IsPrometheusBestPracticeName(notSnakeCase))
	require.True(t, IsPrometheusBestPracticeName(simpleSnakeCase))
	require.True(t, IsPrometheusBestPracticeName(complexSnakeCase))
	require.False(t, IsPrometheusBestPracticeName(leadingSlash))
	require.False(t, IsPrometheusBestPracticeName(trailingSlash))
}
