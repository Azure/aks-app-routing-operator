package suites

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
)

func Test_hydrateExternalDNSSecret(t *testing.T) {
	type args struct {
		ctx            context.Context
		secret         *corev1.Secret
		sp             clients.ServicePrincipal
		subscriptionId string
		tenantId       string
		resourceGroup  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    *corev1.Secret
	}{
		{
			name: "hydrateExternalDNSSecret",
			args: args{
				ctx:    context.Background(),
				secret: &corev1.Secret{},
				sp: clients.ServicePrincipal{
					ApplicationClientID:          "applicationClientID-test",
					ServicePrincipalCredPassword: "servicePrincipalCredPassword-test",
				},
				subscriptionId: "subscriptionId",
				tenantId:       "tenantId",
				resourceGroup:  "resourceGroup",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.args.secret
			if err := hydrateExternalDNSSecret(tt.args.ctx, tt.args.secret, tt.args.sp, tt.args.subscriptionId, tt.args.tenantId, tt.args.resourceGroup); (err != nil) != tt.wantErr {
				t.Errorf("hydrateExternalDNSSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if s.Data["azure.json"] == nil {
				t.Error("azure.json not found in secret")
			}
			json64 := s.Data["azure.json"]
			azureJson, err := base64.StdEncoding.DecodeString(string(json64))
			if err != nil {
				t.Errorf("Error decoding azure.json: %v", err)
			}
			az := &ExternalDNSAzureJson{}
			err = json.Unmarshal(azureJson, az)
			if err != nil {
				t.Errorf("Error unmarshalling azure.json: %v", err)
			}
			if az.SubscriptionId != tt.args.subscriptionId {
				t.Errorf("SubscriptionID not set correctly: %v", az.SubscriptionId)
			}
			if az.TenantId != tt.args.tenantId {
				t.Errorf("TenantId not set correctly: %v", az.TenantId)
			}
			if az.ResourceGroup != tt.args.resourceGroup {
				t.Errorf("ResourceGroup not set correctly: %v", az.ResourceGroup)
			}
			if az.AadClientId != tt.args.sp.ApplicationClientID {
				t.Errorf("ClientId not set correctly: %v", az.AadClientId)
			}
			if az.AadClientSecret != tt.args.sp.ServicePrincipalCredPassword {
				t.Errorf("ClientSecret not set correctly: %v", az.AadClientSecret)
			}

		})
	}
}
