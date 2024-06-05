// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var nginxExceptions = []GatekeeperException{
	{
		// we can't set limits on nginx pods because nginx chooses workers based on Node specs. No "right size fits all".
		// this is also the official recommendation of ingress-nginx.
		MessageSuffix: "container <controller> has no resource limits",
		Constraint:    "container-must-have-limits",
	},
}

var (
	ingConfig = &NginxIngressConfig{
		ControllerClass: "webapprouting.kubernetes.azure.com/nginx",
		ResourceName:    "nginx",
		IcName:          "webapprouting.kubernetes.azure.com",
		ServiceConfig: &ServiceConfig{
			Annotations: map[string]string{
				"service.beta.kubernetes.io/azure-load-balancer-internal": "true",
			},
		},
		MinReplicas:                    2,
		MaxReplicas:                    100,
		TargetCPUUtilizationPercentage: 80,
	}
	controllerTestCases = []struct {
		Name      string
		Conf      *config.Config
		Deploy    *appsv1.Deployment
		IngConfig *NginxIngressConfig
	}{
		{
			Name: "full",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: ingConfig,
		},
		{
			Name: "no-ownership",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			IngConfig: ingConfig,
		},
		{
			Name: "kube-system",
			Conf: &config.Config{
				NS:          "kube-system",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			IngConfig: ingConfig,
		},
		{
			Name: "optional-features-disabled",
			Conf: &config.Config{
				NS:              "test-namespace",
				Registry:        "test-registry",
				MSIClientID:     "test-msi-client-id",
				TenantID:        "test-tenant-id",
				Cloud:           "test-cloud",
				Location:        "test-location",
				DisableKeyvault: true,
				DisableOSM:      true,
			},
			IngConfig: ingConfig,
		},
		{
			Name: "internal",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: &NginxIngressConfig{
				ControllerClass:                "test-controller-class",
				ResourceName:                   "nginx",
				IcName:                         "nginx-private",
				MinReplicas:                    2,
				MaxReplicas:                    100,
				TargetCPUUtilizationPercentage: 80,
			},
		},
		{
			Name: "internal-with-ssl-cert",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: &NginxIngressConfig{
				ControllerClass:                "test-controller-class",
				ResourceName:                   "nginx",
				IcName:                         "nginx-private",
				DefaultSSLCertificate:          "fakenamespace/fakename",
				MinReplicas:                    2,
				MaxReplicas:                    100,
				TargetCPUUtilizationPercentage: 80,
			},
		},
		{
			Name: "internal-with-ssl-cert-and-force-ssl",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: &NginxIngressConfig{
				ControllerClass:                "test-controller-class",
				ResourceName:                   "nginx",
				IcName:                         "nginx-private",
				DefaultSSLCertificate:          "fakenamespace/fakename",
				ForceSSLRedirect:               true,
				MinReplicas:                    2,
				MaxReplicas:                    100,
				TargetCPUUtilizationPercentage: 80,
			},
		},
		{
			Name: "internal-with-nil-ssl-cert-and-force-ssl",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: &NginxIngressConfig{
				ControllerClass:                "test-controller-class",
				ResourceName:                   "nginx",
				IcName:                         "nginx-private",
				DefaultSSLCertificate:          "",
				ForceSSLRedirect:               true,
				MinReplicas:                    2,
				MaxReplicas:                    100,
				TargetCPUUtilizationPercentage: 80,
			},
		},
		{
			Name: "full-with-replicas",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: func() *NginxIngressConfig {
				copy := *ingConfig
				copy.MinReplicas = 15
				copy.MaxReplicas = 30
				copy.TargetCPUUtilizationPercentage = 80
				return &copy
			}(),
		},
		{
			Name: "full-with-target-cpu",
			Conf: &config.Config{
				NS:          "test-namespace",
				Registry:    "test-registry",
				MSIClientID: "test-msi-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "test-cloud",
				Location:    "test-location",
			},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: func() *NginxIngressConfig {
				copy := *ingConfig
				copy.MinReplicas = 15
				copy.MaxReplicas = 30
				copy.TargetCPUUtilizationPercentage = 63
				return &copy
			}(),
		},
	}
	classTestCases = []struct {
		Name      string
		Conf      *config.Config
		Deploy    *appsv1.Deployment
		IngConfig *NginxIngressConfig
	}{
		{
			Name: "full",
			Conf: &config.Config{NS: "test-namespace"},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: ingConfig,
		},
		{
			Name:      "no-ownership",
			Conf:      &config.Config{NS: "test-namespace"},
			IngConfig: ingConfig,
		},
	}
)

func TestIngressControllerResources(t *testing.T) {
	for _, tc := range controllerTestCases {
		objs := GetNginxResources(tc.Conf, tc.IngConfig)
		fixture := path.Join("fixtures", "nginx", tc.Name) + ".yaml"
		AssertFixture(t, fixture, objs.Objects())
		GatekeeperTest(t, fixture, nginxExceptions...)
	}
}

func TestMapAdditions(t *testing.T) {
	testMap := map[string]string{"testkey1": "testval1"}
	withAdditions := AddComponentLabel(testMap, "ingress-controller")

	if withAdditions["testkey1"] != "testval1" {
		t.Errorf("new map doesn't include original values")
	}

	if withAdditions["app.kubernetes.io/component"] != "ingress-controller" {
		t.Errorf("new map doesn't include correct labels for ingress controller deployment")
	}

	_, ok := testMap["app.kubernetes.io/component"]
	if ok {
		t.Errorf("original map was written to")
	}
}
