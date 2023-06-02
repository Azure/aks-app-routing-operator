package fixtures

import (
	"fmt"
	"sigs.k8s.io/e2e-framework/klient/k8s"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// these need to be the same as the consts in promclient/main.go
const (
	PromServer = "prometheus-server"
	promNsEnv  = "PROM_NS"
)

const (
	promConfig     = "prometheus-configuration"
	promConfigFile = "prometheus.yaml"
)

func NewPrometheusClient(ns, image string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prometheus-client",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "prometheus-client"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "prometheus-client"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "prometheus-client",
						Image: image,
						Env: []corev1.EnvVar{{
							Name:  promNsEnv,
							Value: ns,
						}},
						ReadinessProbe: &corev1.Probe{
							FailureThreshold:    1,
							InitialDelaySeconds: 1,
							PeriodSeconds:       1,
							SuccessThreshold:    1,
							TimeoutSeconds:      10,
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path:   "/",
									Port:   intstr.FromInt(8080),
									Scheme: corev1.URISchemeHTTP,
								},
							},
						},
					}},
				},
			},
		},
	}
}

// NewPrometheus returns objects for running Prometheus that monitors web app routing
func NewPrometheus(ns string) []k8s.Object {
	// comes from https://github.com/kubernetes/ingress-nginx/blob/main/deploy/prometheus/prometheus.yaml
	// just a standard prometheus config for nginx-ingress
	c := fmt.Sprintf(`
global:
  scrape_interval: 10s
scrape_configs:
- job_name: 'nginx-ingress'
  kubernetes_sd_configs:
  - role: pod
    namespaces:
      names:
      - %s
  relabel_configs:
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
    action: keep
    regex: true
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scheme]
    action: replace
    target_label: __scheme__
    regex: (https?)
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
    action: replace
    target_label: __metrics_path__
    regex: (.+)
  - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
    action: replace
    target_label: __address__
    regex: ([^:]+)(?::\d+)?;(\d+)
    replacement: $1:$2
  - source_labels: [__meta_kubernetes_service_name]
    regex: %s
    action: drop
`, config.DefaultNs, PromServer)

	return []k8s.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      promConfig,
				Namespace: ns,
			},
			Data: map[string]string{
				promConfigFile: c,
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PromServer,
				Namespace: ns,
			},
		},
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRole",
				APIVersion: "rbac.authorization.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: PromServer,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services", "endpoints", "pods"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRoleBinding",
				APIVersion: "rbac.authorization.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      PromServer,
				Namespace: ns,
			},
			Subjects: []rbacv1.Subject{
				{
					Name:      PromServer,
					Namespace: ns,
					Kind:      "ServiceAccount",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name:     PromServer,
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
			},
		},
		&appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      PromServer,
				Namespace: ns,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: util.Int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": PromServer}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": PromServer}},
					Spec: corev1.PodSpec{
						ServiceAccountName: PromServer,
						Containers: []corev1.Container{
							{
								Name:  "prometheus",
								Image: "prom/prometheus",
								Args: []string{
									"--config.file=/etc/prometheus/" + promConfigFile,
									"--storage.tsdb.path=/prometheus/",
								},
								Ports: []corev1.ContainerPort{
									{ContainerPort: 9090},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "prometheus-config-volume",
										MountPath: "/etc/prometheus/",
									},
									{
										Name:      "prometheus-storage-volume",
										MountPath: "/prometheus/",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "prometheus-config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: promConfig,
										},
									},
								},
							},
							{
								Name: "prometheus-storage-volume",
								// empty volume, it loses memory if pod is terminated
							},
						},
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PromServer,
				Namespace: ns,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": PromServer},
				Type:     "NodePort",
				Ports: []corev1.ServicePort{
					{
						Port:       9090,
						TargetPort: intstr.Parse("9090"),
					},
				},
			},
		},
	}
}
