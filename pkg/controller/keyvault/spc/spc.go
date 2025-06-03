package spc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

type spcOpts struct {
	// clientId is the identity client ID that will be used to access Keyvault
	clientId string
	// tenantId is the tenant ID of the Keyvault
	tenantId string
	// vaultName is the name of the Keyvault that contains the certificate
	vaultName string
	// certName is the name of the certificate in Keyvault
	certName string
	// objectVersion is the version of the secret in Keyvault, if empty, the latest version will be used
	objectVersion string
	// secretName is the name of the secret in Kubernetes that will be created by the SPC
	secretName string
	// cloud is the cloud environment to use, if empty, the default Azure cloud will be used
	cloud string
}

type secretProviderClassReconciler[objectType client.Object] struct {
	// config options
	name            controllername.ControllerNamer
	spcNamer        func(objectType) string
	shouldReconcile func(logr.Logger, client.Client) bool
	toSpcOpts       func(objectType) (spcOpts, error)

	// set during constructor
	client client.Client
	events record.EventRecorder
	config *config.Config
}

func (s *secretProviderClassReconciler[objectType]) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	defer func() {
		metrics.HandleControllerReconcileMetrics(s.name, result, retErr)
	}()

	logger, err := logr.FromContext(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting logger from context: %w", err)
	}
	logger = s.name.AddToLogger(logger).WithValues("name", req.Name, "namespace", req.Namespace)

	logger.Info("getting Object")
	obj := *new(objectType)
	if err := s.client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("generation", obj.GetGeneration())

	// todo: verify that the api version here is okay
	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.spcNamer(obj),
			Namespace: obj.GetNamespace(),
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
				Controller: util.ToPtr(true),
				Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
				Name:       obj.GetName(),
				UID:        obj.GetUID(),
			}},
		},
		// we build the spec later, after we determine if we should reconcile and get the options
	}
	logger = logger.WithValues("spc", spc.Name)

	if !s.shouldReconcile(logger, s.client) {
		logger.Info("skipping reconciliation for SecretProviderClass, will attempt to cleanup")

		logger.Info("getting SecretProviderClass to clean")
		toCleanSPC := &secv1.SecretProviderClass{}
		if err := s.client.Get(ctx, client.ObjectKeyFromObject(spc), toCleanSPC); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "failed to get SecretProviderClass to clean")
				return ctrl.Result{}, fmt.Errorf("getting SecretProviderClass to clean: %w", err)
			}

			logger.Info("SecretProviderClass not found, nothing to clean")
			return ctrl.Result{}, nil
		}

		if manifests.HasTopLevelLabels(toCleanSPC.Labels) {
			logger.Info("deleting SecretProviderClass")
			if err := s.client.Delete(ctx, toCleanSPC); err != nil {
				logger.Error(err, "failed to delete SecretProviderClass")
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}

			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		logger.Info("SecretProviderClass does not have top-level labels, not managed by us. Fully skipping.")
		return ctrl.Result{}, nil
	}

	spcOpts, err := s.toSpcOpts(obj)
	if err != nil {
		var userErr util.UserError
		if errors.As(err, &userErr) {
			logger.Info(fmt.Sprintf("failed to build secret provider class with user error: %s", userErr.Error()))
			s.events.Eventf(obj, corev1.EventTypeWarning, "InvalidInput", "error while processing Keyvault reference: %s", userErr.UserError())
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to get SPC options")
		return ctrl.Result{}, fmt.Errorf("getting SPC options: %w", err)
	}

	if err := s.buildSpcSpec(spc, spcOpts); err != nil {
		logger.Error(err, "failed to build SecretProviderClass spec")
		return ctrl.Result{}, fmt.Errorf("building SecretProviderClass spec: %w", err)
	}

	logger.Info("reconciling SecretProviderClass")
	if err := util.Upsert(ctx, s.client, spc); err != nil {
		err := fmt.Errorf("failed to reconcile SecretProviderClass %s: %w", spc.Name, err)
		s.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateSPC", "error while creating or updating SecretProviderClass needed to pull Keyvault reference: %s", err.Error())
		logger.Error(err, "failed to upsert SecretProviderClass")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (s *secretProviderClassReconciler[objectType]) buildSpcSpec(spc *secv1.SecretProviderClass, opts spcOpts) error {
	p := map[string]interface{}{
		"objectName": opts.certName,
		"objectType": "secret",
	}
	if opts.objectVersion != "" {
		p["objectVersion"] = opts.objectVersion
	}

	params, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshalling parameters: %w", err)
	}

	objects, err := json.Marshal(map[string]interface{}{"array": []string{string(params)}})
	if err != nil {
		return fmt.Errorf("marshalling objects: %w", err)
	}

	spc.Spec = secv1.SecretProviderClassSpec{
		Provider: secv1.Provider("azure"),
		SecretObjects: []*secv1.SecretObject{{
			SecretName: opts.secretName,
			Type:       "kubernetes.io/tls",
			Data: []*secv1.SecretObjectData{
				{
					ObjectName: opts.certName,
					Key:        "tls.key",
				},
				{
					ObjectName: opts.certName,
					Key:        "tls.crt",
				},
			},
		}},
		// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
		Parameters: map[string]string{
			"keyvaultName":           opts.vaultName,
			"useVMManagedIdentity":   "true",
			"userAssignedIdentityID": opts.clientId,
			"tenantId":               opts.tenantId,
			"objects":                string(objects),
		},
	}

	if opts.cloud != "" {
		spc.Spec.Parameters["cloud"] = opts.cloud
	}

	return nil
}
