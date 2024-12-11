package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyAndSetCondition(t *testing.T) {
	// new condition
	nic := validNginxIngressController()
	nic.Generation = 456

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	VerifyAndSetCondition(&nic, cond)
	got := nic.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set same condition
	nic.Generation = 789
	VerifyAndSetCondition(&nic, cond)
	got = nic.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	VerifyAndSetCondition(&nic, cond2)
	got = nic.GetCondition(cond2.Type)
	require.NotNil(t, got)
	require.Equal(t, cond2.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond2.Reason, got.Reason)
	require.Equal(t, cond2.Message, got.Message)

	// old condition should not be changed
	got = nic.GetCondition(cond.Type)
	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)
}
