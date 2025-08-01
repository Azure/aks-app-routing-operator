package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeConditioner struct {
	conditions []metav1.Condition
	generation int64
}

func (f *fakeConditioner) GetCondition(t string) *metav1.Condition {
	return meta.FindStatusCondition(f.conditions, t)
}

func (f *fakeConditioner) GetConditions() *[]metav1.Condition {
	return &f.conditions
}

func (f *fakeConditioner) GetGeneration() int64 {
	return f.generation
}

func TestVerifyAndSetCondition(t *testing.T) {
	// new condition
	f := fakeConditioner{
		generation: 456,
	}

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	VerifyAndSetCondition(&f, cond)
	got := f.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, f.generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set same condition
	f.generation = 789
	VerifyAndSetCondition(&f, cond)
	got = f.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, f.generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	VerifyAndSetCondition(&f, cond2)
	got = f.GetCondition(cond2.Type)
	require.NotNil(t, got)
	require.Equal(t, cond2.Status, got.Status)
	require.Equal(t, f.generation, got.ObservedGeneration)
	require.Equal(t, cond2.Reason, got.Reason)
	require.Equal(t, cond2.Message, got.Message)

	// old condition should not be changed
	got = f.GetCondition(cond.Type)
	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, f.generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)
}
