// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package fixtures

import (
	_ "embed"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

//go:embed client/main.go
var clientContents string

//go:embed server/main.go
var serverContents string

type DeploymentType int

const (
	Client DeploymentType = iota
	Server
)

func (d DeploymentType) String() string {
	return []string{"client", "server"}[d]
}

func NewClientDeployment(t *testing.T, host string, nameservers []string) *appsv1.Deployment {
	deploy := NewGoDeployment(t, Client)
	deploy.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"
	deploy.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name:  "URL",
		Value: "https://" + host,
	}, {
		Name:  "NAMESERVERS",
		Value: strings.Join(nameservers, ","),
	}, {
		Name:      "POD_IP",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
	}}
	deploy.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
		FailureThreshold:    1,
		InitialDelaySeconds: 1,
		PeriodSeconds:       1,
		SuccessThreshold:    1,
		TimeoutSeconds:      5,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/",
				Port:   intstr.FromInt(8080),
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}
	return deploy
}

func NewGoDeployment(t testing.TB, d DeploymentType) *appsv1.Deployment {
	var source string
	switch d {
	case Client:
		source = clientContents
	case Server:
		source = serverContents
	default:
		t.Fatalf("test failed: invalid deployment name given")
	}

	name := d.String()

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": name},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "container",
						Image: "mcr.microsoft.com/oss/go/microsoft/golang:1.20",
						Command: []string{
							"/bin/sh",
							"-c",
							"mkdir source && cd source && go mod init source && echo '" + source + "' > main.go && go run main.go",
						},
					}},
				},
			},
		},
	}
}

func NewService(app, host, keyvaultURI string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: app,
			Annotations: map[string]string{
				"kubernetes.azure.com/ingress-host":          host,
				"kubernetes.azure.com/tls-cert-keyvault-uri": keyvaultURI,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       1234, // anything - we don't use this one
				TargetPort: intstr.FromInt(int(port)),
			}},
			Selector: map[string]string{"app": app},
		},
	}
}
