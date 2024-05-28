package config

import (
	"errors"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
)

func TestControllerConfigString(t *testing.T) {
	cases := []struct {
		name     string
		val      *ControllerConfig
		expected string
	}{
		{
			name:     "nil",
			val:      nil,
			expected: "nil",
		},
		{
			name:     "standard",
			val:      util.ToPtr(Standard),
			expected: "standard",
		},
		{
			name:     "public",
			val:      util.ToPtr(Public),
			expected: "public",
		},
		{
			name:     "private",
			val:      util.ToPtr(Private),
			expected: "private",
		},
		{
			name:     "off",
			val:      util.ToPtr(Off),
			expected: "off",
		},
		{
			name:     "casted type",
			val:      util.ToPtr(ControllerConfig(200)),
			expected: "unknown",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.val.String()
			require.Equal(t, c.expected, got)
		})
	}
}

func TestControllerConfigSet(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		expectedConfig ControllerConfig
		expectedErr    error
	}{
		{
			name:           "empty",
			input:          "",
			expectedConfig: Standard,
		},
		{
			name:           "unknown",
			input:          "unknown",
			expectedConfig: Standard,
			expectedErr:    errors.New("controller config value not recognized"),
		},
		{
			name:           "standard",
			input:          "standard",
			expectedConfig: Standard,
		},
		{
			name:           "public",
			input:          "public",
			expectedConfig: Public,
		},
		{
			name:           "private",
			input:          "private",
			expectedConfig: Private,
		},
		{
			name:           "off",
			input:          "off",
			expectedConfig: Off,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cc ControllerConfig
			ccPointer := util.ToPtr(cc)
			err := ccPointer.Set(c.input)

			require.Equal(t, c.expectedConfig, *ccPointer)
			require.Equal(t, c.expectedErr, err)
		})
	}
}
