// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"path"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/go-logr/logr"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	placeholderPodControllerName = controllername.New("keyvault", "placeholder", "pod")
)

const (
	keyvaultServicePrincipalSecretName = "keyvault-service-principal"
)

// PlaceholderPodController manages a single-replica deployment of no-op pods that mount the
// Keyvault secrets referenced by each secret provider class managed by IngressSecretProviderClassReconciler.
//
// This is necessitated by the Keyvault CSI implementation, which requires at least one mount
// in order to start mirroring the Keyvault values into corresponding Kubernetes secret(s).
type PlaceholderPodController struct {
	client         client.Client
	config         *config.Config
	ingressManager IngressManager
}

func NewPlaceholderPodController(manager ctrl.Manager, conf *config.Config, ingressManager IngressManager) error {
	metrics.InitControllerMetrics(placeholderPodControllerName)
	if conf.DisableKeyvault {
		return nil
	}
	return placeholderPodControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&secv1.SecretProviderClass{}), manager.GetLogger(),
	).Complete(&PlaceholderPodController{client: manager.GetClient(), config: conf, ingressManager: ingressManager})
}

func (p *PlaceholderPodController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}

	// do metrics
	defer func() {
		//placing this call inside a closure allows for result and err to be bound after Reconcile executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(placeholderPodControllerName, result, err)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return result, err
	}
	logger = placeholderPodControllerName.AddToLogger(logger).WithValues("namespace", req.Namespace, "name", req.Name)

	logger.Info("getting secret provider class")
	spc := &secv1.SecretProviderClass{}
	err = p.client.Get(ctx, req.NamespacedName, spc)
	if err != nil {
		return result, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("name", spc.Name, "namespace", spc.Namespace, "generation", spc.Generation)

	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: spc.APIVersion,
				Controller: util.BoolPtr(true),
				Kind:       spc.Kind,
				Name:       spc.Name,
				UID:        spc.UID,
			}},
		},
	}
	logger = logger.WithValues("deployment", dep.Name)

	ing := &netv1.Ingress{}
	ing.Name = util.FindOwnerKind(spc.OwnerReferences, "Ingress")
	ing.Namespace = req.Namespace
	logger = logger.WithValues("ingress", ing.Name)
	if ing.Name != "" {
		logger.Info("getting owner ingress")
		if err = p.client.Get(ctx, client.ObjectKeyFromObject(ing), ing); err != nil {
			return result, client.IgnoreNotFound(err)
		}
	}

	managed := p.ingressManager.IsManaging(ing)
	if ing.Name == "" || ing.Spec.IngressClassName == nil || !managed {
		logger.Info("cleaning unused placeholder pod deployment")

		logger.Info("getting placeholder deployment")
		if err = p.client.Get(ctx, client.ObjectKeyFromObject(dep), dep); err != nil {
			return result, client.IgnoreNotFound(err)
		}

		logger.Info("deleting placeholder deployment")
		err = p.client.Delete(ctx, dep)
		return result, client.IgnoreNotFound(err)
	}

	if p.config.EnableServicePrincipal {
		sec := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      keyvaultServicePrincipalSecretName,
				Namespace: req.Namespace,
			},
		}
		if err = util.Upsert(ctx, p.client, sec); err != nil {
			return result, err
		}
	}

	// Manage a deployment resource
	logger.Info("reconciling placeholder deployment for secret provider class")
	p.buildDeployment(dep, spc, ing)
	if err = util.Upsert(ctx, p.client, dep); err != nil {
		return result, err
	}

	return result, nil
}

func (p *PlaceholderPodController) buildDeployment(dep *appsv1.Deployment, spc *secv1.SecretProviderClass, ing *netv1.Ingress) {
	labels := map[string]string{"app": spc.Name}
	v := corev1.Volume{
		Name: "secrets",
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:           "secrets-store.csi.k8s.io",
				ReadOnly:         util.BoolPtr(true),
				VolumeAttributes: map[string]string{"secretProviderClass": spc.Name},
			},
		},
	}
	if p.config.EnableServicePrincipal {
		v.CSI.NodePublishSecretRef = &corev1.LocalObjectReference{
			Name: keyvaultServicePrincipalSecretName,
		}
	}
	dep.Spec = appsv1.DeploymentSpec{
		Replicas:             util.Int32Ptr(1),
		RevisionHistoryLimit: util.Int32Ptr(2),
		Selector:             &metav1.LabelSelector{MatchLabels: labels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					"kubernetes.azure.com/observed-generation": strconv.FormatInt(spc.Generation, 10),
					"kubernetes.azure.com/purpose":             "hold CSI mount to enable keyvault-to-k8s secret mirroring",
					"kubernetes.azure.com/ingress-owner":       ing.Name,
					"openservicemesh.io/sidecar-injection":     "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				AutomountServiceAccountToken: util.BoolPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: path.Join(p.config.Registry, "/oss/kubernetes/pause:3.6-hotfix.20220114"),
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "secrets",
						MountPath: "/mnt/secrets",
						ReadOnly:  true,
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("20m"),
							corev1.ResourceMemory: resource.MustParse("24Mi"),
						},
					},
				}},
				Volumes: []corev1.Volume{v},
			}),
		},
	}
}
