// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	spcpkg "github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault/spc"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
)

var placeholderPodControllerName = controllername.New("keyvault", "placeholder", "pod")

// PlaceholderPodController manages a single-replica deployment of no-op pods that mount the
// Keyvault secrets referenced by each secret provider class managed by IngressSecretProviderClassReconciler.
//
// This is necessitated by the Keyvault CSI implementation, which requires at least one mount
// in order to start mirroring the Keyvault values into corresponding Kubernetes secret(s).
type PlaceholderPodController struct {
	client         client.Client
	events         record.EventRecorder
	config         *config.Config
	ingressManager util.IngressManager
}

func NewPlaceholderPodController(manager ctrl.Manager, conf *config.Config, ingressManager util.IngressManager) error {
	metrics.InitControllerMetrics(placeholderPodControllerName)
	if conf.DisableKeyvault {
		return nil
	}
	return placeholderPodControllerName.AddToController(
		ctrl.
			NewControllerManagedBy(manager).
			For(&secv1.SecretProviderClass{}), manager.GetLogger(),
	).Complete(&PlaceholderPodController{
		client:         manager.GetClient(),
		config:         conf,
		ingressManager: ingressManager,
		events:         manager.GetEventRecorderFor("aks-app-routing-operator"),
	})
}

func (p *PlaceholderPodController) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, retErr error) {
	// do metrics
	defer func() {
		// placing this call inside a closure allows for result and err to be bound after Reconcile executes
		// this makes sure they have the proper value
		// just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		// the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(placeholderPodControllerName, res, retErr)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating logger: %w", err)
	}
	logger = placeholderPodControllerName.AddToLogger(logger).WithValues("namespace", req.Namespace, "name", req.Name)

	logger.Info("getting secret provider class")
	spc := &secv1.SecretProviderClass{}
	err = p.client.Get(ctx, req.NamespacedName, spc)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to fetch SPC: %s", err.Error())
			return ctrl.Result{}, fmt.Errorf("fetching SPC: %w", err)
		}
		return ctrl.Result{}, nil
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
			Labels:    spc.Labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: spc.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       spc.Kind,
				Name:       spc.Name,
				UID:        spc.UID,
			}},
		},
	}
	logger = logger.WithValues("deployment", dep.Name)

	return p.reconcileObjectDeployment(dep, spc, req, ctx, logger)
}

func (p *PlaceholderPodController) reconcileObjectDeployment(dep *appsv1.Deployment, spc *secv1.SecretProviderClass, req ctrl.Request, ctx context.Context, logger logr.Logger) (ctrl.Result, error) {
	var (
		err            error
		obj            client.Object
		serviceAccount string
	)

	result := ctrl.Result{}

	switch {
	case util.FindOwnerKind(spc.OwnerReferences, "NginxIngressController") != "":
		obj = &v1alpha1.NginxIngressController{}
		obj.SetName(util.FindOwnerKind(spc.OwnerReferences, "NginxIngressController"))
		logger.Info(fmt.Sprint("getting owner NginxIngressController"))
	case util.FindOwnerKind(spc.OwnerReferences, "Ingress") != "":
		obj = &netv1.Ingress{}
		obj.SetName(util.FindOwnerKind(spc.OwnerReferences, "Ingress"))
		obj.SetNamespace(req.Namespace)
		logger.Info(fmt.Sprint("getting owner Ingress"))
	case util.FindOwnerKind(spc.OwnerReferences, "Gateway") != "":
		obj = &gatewayv1.Gateway{}
		gwName := util.FindOwnerKind(spc.OwnerReferences, "Gateway")
		obj.SetName(gwName)
		obj.SetNamespace(req.Namespace)
		logger.Info(fmt.Sprintf("getting owner Gateway resource %s", gwName))
	default:
		logger.Info("owner type not found")
		return result, nil
	}

	if obj.GetName() != "" {
		if err = p.client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return result, client.IgnoreNotFound(err)
		}
	}

	cleanPod, err := p.placeholderPodCleanCheck(spc, obj)
	if err != nil {
		return result, err
	}

	if cleanPod {
		logger.Info("attempting to clean unused placeholder pod deployment")
		logger.Info("getting placeholder deployment")
		toCleanDeployment := &appsv1.Deployment{}
		if err = p.client.Get(ctx, client.ObjectKeyFromObject(dep), toCleanDeployment); err != nil {
			return result, client.IgnoreNotFound(err)
		}
		if manifests.HasTopLevelLabels(toCleanDeployment.Labels) {
			logger.Info("deleting placeholder deployment")
			err = p.client.Delete(ctx, toCleanDeployment)
			return result, client.IgnoreNotFound(err)
		}

		logger.Info("deployment found but it's not managed by us, skipping cleaning")
		return result, nil
	}

	// Verify ServiceAccount exists (if Gateway)
	serviceAccount, err = p.verifyServiceAccount(ctx, spc, obj, logger)
	if err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info("user error while verifying if service account exists: %s", userErr.Err)
			p.events.Eventf(obj, corev1.EventTypeWarning, "InvalidInput", userErr.UserMessage)
			return result, nil
		}

		logger.Error(err, "verifying ServiceAccount for placeholder pod")
		return result, fmt.Errorf("verifying service account for placeholder pod: %s", err.Error())
	}

	// Manage a deployment resource
	logger.Info("reconciling placeholder deployment for secret provider class")
	if err = p.buildDeployment(ctx, dep, spc, obj); err != nil {
		err = fmt.Errorf("building deployment: %w", err)
		p.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreatePlaceholderPodDeployment", "error while building placeholder pod Deployment needed to pull Keyvault reference: %s", err.Error())
		logger.Error(err, "failed to build placeholder deployment")
		return result, err
	}

	if serviceAccount != "" {
		dep.Spec.Template.Spec.AutomountServiceAccountToken = to.Ptr(true)
		dep.Spec.Template.Spec.ServiceAccountName = serviceAccount
	}

	if err = util.Upsert(ctx, p.client, dep); err != nil {
		p.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreatePlaceholderPodDeployment", "error while creating or updating placeholder pod Deployment needed to pull Keyvault reference: %s", err.Error())
		logger.Error(err, "failed to upsert placeholder deployment")
		return result, err
	}

	return result, nil
}

func (p *PlaceholderPodController) placeholderPodCleanCheck(spc *secv1.SecretProviderClass, obj client.Object) (bool, error) {
	switch t := obj.(type) {
	case *v1alpha1.NginxIngressController:
		return !spcpkg.ShouldReconcileNic(t), nil
	case *netv1.Ingress:
		managed, err := spcpkg.ShouldReconcileIngress(p.ingressManager, t)
		if err != nil {
			return false, fmt.Errorf("determining if ingress is managed: %w", err)
		}

		return !managed, nil
	case *gatewayv1.Gateway:
		if !spcpkg.IsManagedGateway(t) {
			return true, nil
		}

		for _, listener := range t.Spec.Listeners {
			if spc.Name != spcpkg.GetGatewayListenerSpcName(t.Name, string(listener.Name)) {
				continue
			}

			return !spcpkg.ListenerIsKvEnabled(listener), nil
		}

		// couldn't find the listener the pod belongs to so return true
		return true, nil
	}

	return false, nil
}

// getCurrentDeployment returns the current deployment for the given name or nil if it does not exist. nil, nil is returned if the deployment is not found
func (p *PlaceholderPodController) getCurrentDeployment(ctx context.Context, name types.NamespacedName) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{}
	err := p.client.Get(ctx, name, dep)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	return dep, nil
}

func (p *PlaceholderPodController) buildDeployment(ctx context.Context, dep *appsv1.Deployment, spc *secv1.SecretProviderClass, obj client.Object) error {
	old, err := p.getCurrentDeployment(ctx, client.ObjectKeyFromObject(dep))
	if err != nil {
		return fmt.Errorf("getting current deployment: %w", err)
	}

	labels := map[string]string{"app": spc.Name}

	if old != nil { // we need to ensure that immutable fields are not changed
		labels = old.Spec.Selector.MatchLabels
	}

	var ownerAnnotation string
	switch obj.(type) {
	case *v1alpha1.NginxIngressController:
		ownerAnnotation = "kubernetes.azure.com/nginx-ingress-controller-owner"
	case *netv1.Ingress:
		ownerAnnotation = "kubernetes.azure.com/ingress-owner"
	case *gatewayv1.Gateway:
		ownerAnnotation = "kubernetes.azure.com/gateway-owner"
	default:
		return fmt.Errorf("failed to build deployment: object type not ingress, nginxingresscontroller, or gateway")
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
					ownerAnnotation:                            obj.GetName(),
					"openservicemesh.io/sidecar-injection":     "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				AutomountServiceAccountToken: util.ToPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: path.Join(p.config.Registry, "/oss/kubernetes/pause:3.10"),
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
					SecurityContext: &corev1.SecurityContext{
						Privileged:               util.ToPtr(false),
						AllowPrivilegeEscalation: util.ToPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot:           util.ToPtr(true),
						RunAsUser:              util.Int64Ptr(65535),
						RunAsGroup:             util.Int64Ptr(65535),
						ReadOnlyRootFilesystem: util.ToPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.ToPtr(true),
							VolumeAttributes: map[string]string{"secretProviderClass": spc.Name},
						},
					},
				}},
			}),
		},
	}
	return nil
}

// verifyServiceAccount ensures that the ServiceAccount used to create the placeholder pod exists
func (p *PlaceholderPodController) verifyServiceAccount(ctx context.Context, spc *secv1.SecretProviderClass, obj client.Object, logger logr.Logger) (string, error) {
	var serviceAccount string

	switch t := obj.(type) {
	case *gatewayv1.Gateway:
		logger.Info("verifying service account referenced by listener exists")
		for _, listener := range t.Spec.Listeners {
			if spc.Name != generateGwListenerCertName(t.Name, listener.Name) {
				continue
			}
			if listener.TLS != nil && listener.TLS.Options != nil {
				serviceAccount = string(listener.TLS.Options[serviceAccountTLSOption])
				break
			}
		}

		if serviceAccount == "" {
			err := fmt.Errorf("failed to locate listener for SPC %s on user's gateway resource", spc.Name)
			return "", util.NewUserError(err, fmt.Sprintf("gateway listener for spc %s doesn't exist or doesn't contain required TLS options", spc.Name))
		}

		_, err := util.GetServiceAccountWorkloadIdentityClient(ctx, p.client, serviceAccount, spc.Namespace)
		if err != nil {
			return "", err
		}
		return serviceAccount, nil
	}

	return "", nil
}
