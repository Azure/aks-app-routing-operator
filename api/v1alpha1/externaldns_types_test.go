package v1alpha1

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func validExternalDNS() *ExternalDNS {
	return &ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: ExternalDNSSpec{
			TenantID: "123e4567-e89b-12d3-a456-426614174000",
			DNSZoneResourceIDs: []string{
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
			},
			ResourceTypes: []string{"ingress", "gateway"},
			Identity: ExternalDNSIdentity{
				ServiceAccount: "test-sa",
			},
		},
	}
}

func TestExternalDNSSetCondition(t *testing.T) {
	// new condition
	edc := validExternalDNS()
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

func TestKubebuilderValidation(t *testing.T) {
	tcs := []struct {
		name          string
		edc           *ExternalDNS
		expectedError error
	}{
		{
			name:          "valid",
			edc:           validExternalDNS(),
			expectedError: nil,
		},
		{
			name: "different resourcegroups",
			edc: &ExternalDNS{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "diff-rg",
					Namespace: "default",
				},
				Spec: ExternalDNSSpec{
					TenantID: "tenant-id",
					DNSZoneResourceIDs: []string{
						"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
						"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test2/providers/Microsoft.network/dnszones/test2",
					},
					ResourceTypes: []string{"ingress", "gateway"},
					Identity: ExternalDNSIdentity{
						ServiceAccount: "test-sa",
					},
				},
			},
			expectedError: errors.New("All items must have the same resource group"),
		},
	}

	scheme := runtime.NewScheme()
	_ = AddToScheme(scheme)

	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	fakeClient, _ := client.New(cfg, client.Options{Scheme: scheme})

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			err := fakeClient.Create(nil, tc.edc)
			if tc.expectedError != nil {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}

}
