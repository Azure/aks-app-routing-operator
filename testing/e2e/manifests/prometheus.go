package manifests

import (
	_ "embed"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	promConfigFile = "prometheus.yaml"
)

//go:embed embedded/prom.go
var promClientContents string

type prometheusResources struct {
	Client *appsv1.Deployment
	Server []client.Object
}

func (t prometheusResources) Objects() []client.Object {
	ret := []client.Object{
		t.Client,
	}
	ret = append(ret, t.Server...)

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}

func PrometheusClientAndServer(namespace, name string) prometheusResources {
	clientDeployment := newGoDeployment(promClientContents, namespace, name+"-client")
	clientDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{
			Name:  "PROM_NS",
			Value: namespace,
		},
		{
			Name:  "PROM_NAME",
			Value: name + "-server",
		},
	}
	clientDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
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
`, managedResourceNs, name)

	server := []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-server",
				Namespace: namespace,
			},
			Data: map[string]string{
				promConfigFile: c,
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-server",
				Namespace: namespace,
			},
		},
		&rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRole",
				APIVersion: "rbac.authorization.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name + "-server",
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
				Name:      name + "-server",
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Name:      name + "-server",
					Namespace: namespace,
					Kind:      "ServiceAccount",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name:     name + "-server",
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
				Name:      name + "-server",
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: to.Ptr(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name + "-server"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name + "-server"}},
					Spec: corev1.PodSpec{
						ServiceAccountName: name + "-server",
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
											Name: name + "-server",
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
				Name:      name + "-server",
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": name + "-server"},
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

	return prometheusResources{
		Client: clientDeployment,
		Server: server,
	}
}
