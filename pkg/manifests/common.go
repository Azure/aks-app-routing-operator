package manifests

import (
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

const operatorName = "aks-app-routing-operator"

// GetTopLevelLabels returns labels that every resource App Routing manages have
func GetTopLevelLabels() map[string]string { // this is a function to avoid any accidental mutation due to maps being reference types
	return map[string]string{"app.kubernetes.io/managed-by": operatorName, "kubernetes.azure.com/managedby": "aks"}
}

// HasTopLevelLabels returns true if the given labels match the top level labels
func HasTopLevelLabels(objLabels map[string]string) bool {
	if len(objLabels) == 0 {
		return false
	}

	for label, val := range GetTopLevelLabels() {
		spcVal, ok := objLabels[label]
		if !ok {
			return false
		}
		if spcVal != val {
			return false
		}
	}
	return true
}

// GetOwnerRefs returns the owner references for the given object
func GetOwnerRefs(owner client.Object, controller bool) []metav1.OwnerReference {
	gvk := owner.GetObjectKind().GroupVersionKind()
	apiVersion := gvk.GroupVersion().String()
	kind := gvk.Kind
	name := owner.GetName()
	uid := owner.GetUID()

	return []metav1.OwnerReference{{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		UID:        uid,
		Controller: util.ToPtr(controller),
	}}
}

func Namespace(conf *config.Config, nsName string) *corev1.Namespace {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			// don't set top-level labels,namespace is not managed by operator
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	if !conf.DisableOSM {
		ns.ObjectMeta.Labels["openservicemesh.io/monitored-by"] = "osm"
	}

	return ns
}

func withPodRefEnvVars(contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()
	copy.Env = append(copy.Env, corev1.EnvVar{
		Name: "POD_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	}, corev1.EnvVar{
		Name: "POD_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	})
	return copy
}

func withTypicalReadinessProbe(port int, contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()

	copy.ReadinessProbe = &corev1.Probe{
		FailureThreshold:    3,
		InitialDelaySeconds: 10,
		PeriodSeconds:       5,
		SuccessThreshold:    1,
		TimeoutSeconds:      1,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/healthz",
				Port:   intstr.FromInt(port),
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}

	return copy
}

func withLivenessProbeMatchingReadinessNewFailureThresh(contain *corev1.Container, failureThresh int32) *corev1.Container {
	copy := withLivenessProbeMatchingReadiness(contain)

	if copy != nil && copy.LivenessProbe != nil {
		copy.LivenessProbe.FailureThreshold = failureThresh
	}

	return copy
}

func withLivenessProbeMatchingReadiness(contain *corev1.Container) *corev1.Container {
	copy := contain.DeepCopy()
	copy.LivenessProbe = copy.ReadinessProbe.DeepCopy()
	return copy
}

func WithPreferSystemNodes(spec *corev1.PodSpec) *corev1.PodSpec {
	copy := spec.DeepCopy()
	copy.PriorityClassName = "system-cluster-critical"

	copy.Tolerations = append(copy.Tolerations, corev1.Toleration{
		Key:      "CriticalAddonsOnly",
		Operator: corev1.TolerationOpExists,
	})

	if copy.Affinity == nil {
		copy.Affinity = &corev1.Affinity{}
	}
	if copy.Affinity.NodeAffinity == nil {
		copy.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	copy.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(copy.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, corev1.PreferredSchedulingTerm{
		Weight: 100,
		Preference: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      "kubernetes.azure.com/mode",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"system"},
			}},
		},
	})

	if copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}
	copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(copy.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "kubernetes.azure.com/cluster",
				Operator: corev1.NodeSelectorOpExists,
			},
			{
				Key:      "type",
				Operator: corev1.NodeSelectorOpNotIn,
				Values:   []string{"virtual-kubelet"},
			},
			{
				Key:      "kubernetes.io/os",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"linux"},
			},
		},
	})

	return copy
}

func AddComponentLabel(originalLabels map[string]string, componentName string) map[string]string {
	tr := make(map[string]string)
	for k, v := range originalLabels {
		tr[k] = v
	}
	tr["app.kubernetes.io/component"] = componentName
	return tr
}
