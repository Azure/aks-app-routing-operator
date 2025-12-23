package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHealthChecker implements the healthChecker interface for testing
type mockHealthChecker struct {
	healthy bool
}

func (m *mockHealthChecker) IsHealthy() bool {
	return m.healthy
}

func (m *mockHealthChecker) setHealthy(healthy bool) {
	m.healthy = healthy
}

// TestNewHealthCheckers verifies initialization
func TestNewHealthCheckers(t *testing.T) {
	hc := newHealthCheckers()

	require.NotNil(t, hc)
	assert.NotNil(t, hc.checkers)
	assert.Equal(t, 0, len(hc.checkers))
	assert.True(t, hc.isHealthy(), "should be healthy with no checkers")
}

// TestHealthCheckers_AddCheck_Single verifies adding a single checker
func TestHealthCheckers_AddCheck_Single(t *testing.T) {
	hc := newHealthCheckers()
	checker := &mockHealthChecker{healthy: true}

	hc.addCheck(checker)

	assert.Equal(t, 1, len(hc.checkers))
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_AddCheck_Multiple verifies adding multiple checkers
func TestHealthCheckers_AddCheck_Multiple(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: true}
	checker3 := &mockHealthChecker{healthy: true}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.Equal(t, 3, len(hc.checkers))
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_AddCheck_Nil verifies nil checker is not added
func TestHealthCheckers_AddCheck_Nil(t *testing.T) {
	hc := newHealthCheckers()

	hc.addCheck(nil)

	assert.Equal(t, 0, len(hc.checkers), "nil checker should not be added")
	assert.True(t, hc.isHealthy(), "should still be healthy")
}

// TestHealthCheckers_AddCheck_MultipleNils verifies multiple nil checkers are not added
func TestHealthCheckers_AddCheck_MultipleNils(t *testing.T) {
	hc := newHealthCheckers()
	checker := &mockHealthChecker{healthy: true}

	hc.addCheck(nil)
	hc.addCheck(checker)
	hc.addCheck(nil)
	hc.addCheck(nil)

	assert.Equal(t, 1, len(hc.checkers), "only non-nil checker should be added")
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_IsHealthy_AllHealthy verifies all healthy checkers return healthy
func TestHealthCheckers_IsHealthy_AllHealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: true}
	checker3 := &mockHealthChecker{healthy: true}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.True(t, hc.isHealthy(), "should be healthy when all checkers are healthy")
}

// TestHealthCheckers_IsHealthy_OneUnhealthy verifies one unhealthy checker returns unhealthy
func TestHealthCheckers_IsHealthy_OneUnhealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: false}
	checker3 := &mockHealthChecker{healthy: true}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.False(t, hc.isHealthy(), "should be unhealthy when any checker is unhealthy")
}

// TestHealthCheckers_IsHealthy_AllUnhealthy verifies all unhealthy checkers return unhealthy
func TestHealthCheckers_IsHealthy_AllUnhealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: false}
	checker2 := &mockHealthChecker{healthy: false}
	checker3 := &mockHealthChecker{healthy: false}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.False(t, hc.isHealthy(), "should be unhealthy when all checkers are unhealthy")
}

// TestHealthCheckers_IsHealthy_FirstUnhealthy verifies short-circuit behavior
func TestHealthCheckers_IsHealthy_FirstUnhealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: false}
	checker2 := &mockHealthChecker{healthy: true}
	checker3 := &mockHealthChecker{healthy: true}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.False(t, hc.isHealthy(), "should return false on first unhealthy checker")
}

// TestHealthCheckers_IsHealthy_LastUnhealthy verifies all checkers are evaluated
func TestHealthCheckers_IsHealthy_LastUnhealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: true}
	checker3 := &mockHealthChecker{healthy: false}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.False(t, hc.isHealthy(), "should evaluate all checkers and return unhealthy")
}

// TestHealthCheckers_IsHealthy_EmptyCheckers verifies empty checker list is healthy
func TestHealthCheckers_IsHealthy_EmptyCheckers(t *testing.T) {
	hc := newHealthCheckers()

	assert.True(t, hc.isHealthy(), "empty checker list should be considered healthy")
}

// TestHealthCheckers_IsHealthy_DynamicStateChange verifies dynamic state changes
func TestHealthCheckers_IsHealthy_DynamicStateChange(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: true}

	hc.addCheck(checker1)
	hc.addCheck(checker2)

	// Initially healthy
	assert.True(t, hc.isHealthy())

	// Change state to unhealthy
	checker1.setHealthy(false)
	assert.False(t, hc.isHealthy())

	// Recover health
	checker1.setHealthy(true)
	assert.True(t, hc.isHealthy())

	// Make multiple unhealthy
	checker1.setHealthy(false)
	checker2.setHealthy(false)
	assert.False(t, hc.isHealthy())

	// Partial recovery
	checker1.setHealthy(true)
	assert.False(t, hc.isHealthy(), "should still be unhealthy with one unhealthy checker")

	// Full recovery
	checker2.setHealthy(true)
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_AddCheck_AfterHealthCheck verifies adding checkers after health checks
func TestHealthCheckers_AddCheck_AfterHealthCheck(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}

	// Initial state
	assert.True(t, hc.isHealthy())

	// Add checker and verify
	hc.addCheck(checker1)
	assert.True(t, hc.isHealthy())

	// Add unhealthy checker
	checker2 := &mockHealthChecker{healthy: false}
	hc.addCheck(checker2)
	assert.False(t, hc.isHealthy())

	// Add another healthy checker
	checker3 := &mockHealthChecker{healthy: true}
	hc.addCheck(checker3)
	assert.False(t, hc.isHealthy(), "should remain unhealthy")

	// Fix the unhealthy checker
	checker2.setHealthy(true)
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_OrderIndependence verifies order of checkers doesn't affect result
func TestHealthCheckers_OrderIndependence(t *testing.T) {
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: false}
	checker3 := &mockHealthChecker{healthy: true}

	// Order 1: healthy, unhealthy, healthy
	hc1 := newHealthCheckers()
	hc1.addCheck(checker1)
	hc1.addCheck(checker2)
	hc1.addCheck(checker3)

	// Order 2: unhealthy, healthy, healthy
	hc2 := newHealthCheckers()
	hc2.addCheck(checker2)
	hc2.addCheck(checker1)
	hc2.addCheck(checker3)

	// Order 3: healthy, healthy, unhealthy
	hc3 := newHealthCheckers()
	hc3.addCheck(checker1)
	hc3.addCheck(checker3)
	hc3.addCheck(checker2)

	// All should report unhealthy regardless of order
	assert.False(t, hc1.isHealthy())
	assert.False(t, hc2.isHealthy())
	assert.False(t, hc3.isHealthy())
}

// TestHealthCheckers_SingleChecker verifies behavior with single checker
func TestHealthCheckers_SingleChecker(t *testing.T) {
	tests := []struct {
		name     string
		healthy  bool
		expected bool
	}{
		{
			name:     "single healthy checker",
			healthy:  true,
			expected: true,
		},
		{
			name:     "single unhealthy checker",
			healthy:  false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := newHealthCheckers()
			checker := &mockHealthChecker{healthy: tt.healthy}
			hc.addCheck(checker)

			assert.Equal(t, tt.expected, hc.isHealthy())
		})
	}
}

// TestHealthCheckers_LargeNumberOfCheckers verifies behavior with many checkers
func TestHealthCheckers_LargeNumberOfCheckers(t *testing.T) {
	hc := newHealthCheckers()
	const numCheckers = 1000

	// Add many healthy checkers
	for i := 0; i < numCheckers; i++ {
		checker := &mockHealthChecker{healthy: true}
		hc.addCheck(checker)
	}

	assert.Equal(t, numCheckers, len(hc.checkers))
	assert.True(t, hc.isHealthy())

	// Add one unhealthy checker at the end
	unhealthyChecker := &mockHealthChecker{healthy: false}
	hc.addCheck(unhealthyChecker)

	assert.False(t, hc.isHealthy(), "should be unhealthy with one unhealthy checker among many")

	// Fix the unhealthy checker
	unhealthyChecker.setHealthy(true)
	assert.True(t, hc.isHealthy())
}

// TestHealthCheckers_RepeatedHealthChecks verifies repeated health checks are consistent
func TestHealthCheckers_RepeatedHealthChecks(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: true}
	checker2 := &mockHealthChecker{healthy: false}

	hc.addCheck(checker1)
	hc.addCheck(checker2)

	// Repeated checks should return same result
	for i := 0; i < 100; i++ {
		assert.False(t, hc.isHealthy(), "repeated check %d should be consistent", i)
	}

	// Change state
	checker2.setHealthy(true)

	// Repeated checks should return new result
	for i := 0; i < 100; i++ {
		assert.True(t, hc.isHealthy(), "repeated check %d should be consistent after state change", i)
	}
}

// TestHealthCheckers_ZeroValueChecker verifies behavior with zero-value checker
func TestHealthCheckers_ZeroValueChecker(t *testing.T) {
	hc := newHealthCheckers()
	checker := &mockHealthChecker{} // Zero value, healthy is false

	hc.addCheck(checker)

	assert.False(t, hc.isHealthy(), "zero-value checker should be unhealthy")
}

// TestHealthChecker_Interface_Compliance verifies mock implements interface correctly
func TestHealthChecker_Interface_Compliance(t *testing.T) {
	var _ healthChecker = (*mockHealthChecker)(nil)
}

// TestHealthCheckers_AddNilAndValidMixed verifies mixed nil and valid checker additions
func TestHealthCheckers_AddNilAndValidMixed(t *testing.T) {
	hc := newHealthCheckers()

	hc.addCheck(nil)
	checker1 := &mockHealthChecker{healthy: true}
	hc.addCheck(checker1)
	hc.addCheck(nil)
	checker2 := &mockHealthChecker{healthy: false}
	hc.addCheck(checker2)
	hc.addCheck(nil)
	checker3 := &mockHealthChecker{healthy: true}
	hc.addCheck(checker3)
	hc.addCheck(nil)

	assert.Equal(t, 3, len(hc.checkers), "should only have non-nil checkers")
	assert.False(t, hc.isHealthy(), "should be unhealthy due to checker2")
}

// TestHealthCheckers_RecoverAfterAllUnhealthy verifies recovery from all unhealthy state
func TestHealthCheckers_RecoverAfterAllUnhealthy(t *testing.T) {
	hc := newHealthCheckers()
	checker1 := &mockHealthChecker{healthy: false}
	checker2 := &mockHealthChecker{healthy: false}
	checker3 := &mockHealthChecker{healthy: false}

	hc.addCheck(checker1)
	hc.addCheck(checker2)
	hc.addCheck(checker3)

	assert.False(t, hc.isHealthy(), "should start unhealthy")

	// Recover one at a time
	checker1.setHealthy(true)
	assert.False(t, hc.isHealthy(), "still unhealthy with 2/3 unhealthy")

	checker2.setHealthy(true)
	assert.False(t, hc.isHealthy(), "still unhealthy with 1/3 unhealthy")

	checker3.setHealthy(true)
	assert.True(t, hc.isHealthy(), "should be healthy when all recovered")
}

// TestHealthCheckers_AlternatingStates verifies behavior with alternating health states
func TestHealthCheckers_AlternatingStates(t *testing.T) {
	hc := newHealthCheckers()
	checker := &mockHealthChecker{healthy: true}

	hc.addCheck(checker)

	// Alternate states multiple times
	for i := 0; i < 10; i++ {
		checker.setHealthy(i%2 == 0)
		expected := i%2 == 0
		assert.Equal(t, expected, hc.isHealthy(), "iteration %d should match expected state", i)
	}
}
