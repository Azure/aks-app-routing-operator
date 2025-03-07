package util

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func Test_GetServiceAccountAndVerifyWorkloadIdentity(t *testing.T) {
	tcs := []struct {
		name                string
		saName              string
		namespace           string
		generateClientState func() client.Client
		expectedClientId    string
		expectedError       error
	}{
		{
			name: "nonexistent sa",
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).Build()
			},
			saName:        "test-sa",
			expectedError: errors.New("serviceaccounts \"test-sa\" not found"),
		},
		{
			name:      "cert URI with sa with no annotation",
			namespace: "test-ns",
			saName:    "test-sa",
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "test-ns",
						},
					}).Build()
			},
			expectedClientId: "",
			expectedError:    errors.New("user-specified service account does not contain WI annotation"),
		},
		{
			name:      "cert URI with sa with correct annotation",
			namespace: "test-ns",
			saName:    "test-sa",
			generateClientState: func() client.Client {
				return testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, clientgoscheme.AddToScheme).WithObjects(
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: "v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-sa",
							Namespace: "test-ns",
							Annotations: map[string]string{
								"azure.workload.identity/client-id": "test-client-id",
							},
						},
					}).Build()
			},
			expectedClientId: "test-client-id",
			expectedError:    nil,
		},
	}

	for _, tc := range tcs {
		t.Logf("Running test case: %s", tc.name)
		clientId, err := GetServiceAccountAndVerifyWorkloadIdentity(context.Background(), tc.generateClientState(), tc.saName, tc.namespace)
		if tc.expectedError != nil {
			require.Equal(t, tc.expectedError.Error(), err.Error())
		} else {
			require.Equal(t, nil, err)
			require.Equal(t, tc.expectedClientId, clientId)
		}
	}

}
