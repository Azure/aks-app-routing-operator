package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validExternalDNSConfiguration() *ExternalDNSConfiguration {
	return &ExternalDNSConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: ExternalDNSConfigurationSpec{
			TenantID:           "tenant-id",
			DNSZoneResourceIDs: []string{"dnszone-id", "dnszone-id2"},
			ResourceTypes:      []string{"ingress", "gateway"},
			Identity: ExternalDNSConfigurationIdentity{
				ServiceAccount: "test-sa",
			},
		},
	}
}

func TestExternalDNSConfigurationSetCondition(t *testing.T) {
	// new condition
	edc := validExternalDNSConfiguration()
	edc.Generation = 456

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	edc.SetCondition(cond)
	got := edc.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, edc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set same condition
	edc.Generation = 789
	edc.SetCondition(cond)
	got = edc.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, edc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	edc.SetCondition(cond2)
	got = edc.GetCondition(cond2.Type)
	require.NotNil(t, got)
	require.Equal(t, cond2.Status, got.Status)
	require.Equal(t, edc.Generation, got.ObservedGeneration)
	require.Equal(t, cond2.Reason, got.Reason)
	require.Equal(t, cond2.Message, got.Message)

	// old condition should not be changed
	got = edc.GetCondition(cond.Type)
	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, edc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)
}
