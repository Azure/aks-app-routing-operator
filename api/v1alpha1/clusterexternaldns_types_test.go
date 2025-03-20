package v1alpha1

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validClusterExternalDNS() *ClusterExternalDNS {
	return &ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: ClusterExternalDNSSpec{
			TenantID:           to.Ptr("tenant-id"),
			DNSZoneResourceIDs: []string{"dnszone-id", "dnszone-id2"},
			ResourceTypes:      []string{"ingress", "gateway"},
			Identity: ExternalDNSIdentity{
				ServiceAccount: "test-sa",
			},
			ResourceNamespace: "test-namespace",
		},
	}
}

func TestClusterExternalDNSSetCondition(t *testing.T) {
	// new condition
	cedc := validClusterExternalDNS()
	cedc.Generation = 456

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	cedc.SetCondition(cond)
	got := cedc.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, cedc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set same condition
	cedc.Generation = 789
	cedc.SetCondition(cond)
	got = cedc.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, cedc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	cedc.SetCondition(cond2)
	got = cedc.GetCondition(cond2.Type)
	require.NotNil(t, got)
	require.Equal(t, cond2.Status, got.Status)
	require.Equal(t, cedc.Generation, got.ObservedGeneration)
	require.Equal(t, cond2.Reason, got.Reason)
	require.Equal(t, cond2.Message, got.Message)

	// old condition should not be changed
	got = cedc.GetCondition(cond.Type)
	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, cedc.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)
}
