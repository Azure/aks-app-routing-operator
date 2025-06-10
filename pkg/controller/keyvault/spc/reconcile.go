package spc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// action is used to determine what action to take when reconciling the SecretProviderClass
type action uint

const (
	// actionReconcile means that the SecretProviderClass should be created or updated
	actionReconcile action = iota
	// actionCleanup means that the SecretProviderClass should be deleted if it exists
	actionCleanup
)

type spcOpts struct {
	// action is the action to take when reconciling the SecretProviderClass
	action action
	// name is the name of the SecretProviderClass that will be created
	name string
	// namespace is the namespace of the SecretProviderClass that will be created
	namespace string

	// *** above fields are required, below fields are only needed if action is actionReconcile ***

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

	// if set, the owner object will be updated
	modifyOwner func(obj client.Object) error
}

type secretProviderClassReconciler[objectType client.Object] struct {
	// config options
	name controllername.ControllerNamer
	// toSpcNamer is a function that returns an iterator for each SecretProviderClass that should be managed for the given object
	toSpcOpts func(context.Context, client.Client, objectType) iter.Seq2[spcOpts, error]

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

	objUpdated := false
	for spcOpts, err := range s.toSpcOpts(ctx, s.client, obj) {
		if err != nil {
			var userErr util.UserError
			if errors.As(err, &userErr) {
				logger.Info(fmt.Sprintf("failed to build secret provider class with user error: %s", userErr.Error()))
				s.events.Eventf(obj, corev1.EventTypeWarning, "InvalidInput", "error while processing Keyvault reference: %s", userErr.UserError())
				return ctrl.Result{}, nil
			}

			logger.Error(err, "failed to build secret provider class")
			return ctrl.Result{}, fmt.Errorf("building secret provider class: %w", err)
		}

		if spcOpts.action == actionCleanup {
			if err := s.cleanupSpcOpt(ctx, logger, spcOpts); err != nil {
				logger.Error(err, "failed to clean up SecretProviderClass")
				return ctrl.Result{}, fmt.Errorf("cleaning up SecretProviderClass: %w", err)
			}
		}

		spc, err := s.buildSpc(obj, spcOpts)
		if err != nil {
			logger.Error(err, "failed to build SecretProviderClass spec")
			return ctrl.Result{}, fmt.Errorf("building SecretProviderClass spec: %w", err)
		}
		logger = logger.WithValues("spc", spc.Name)

		logger.Info("reconciling SecretProviderClass")
		if err := util.Upsert(ctx, s.client, spc); err != nil {
			err := fmt.Errorf("failed to reconcile SecretProviderClass %s: %w", spc.Name, err)
			s.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateOrCreateSPC", "error while creating or updating SecretProviderClass needed to pull Keyvault reference: %s", err.Error())
			logger.Error(err, "failed to upsert SecretProviderClass")
			return ctrl.Result{}, err
		}

		if spcOpts.modifyOwner != nil {
			if err := spcOpts.modifyOwner(obj); err != nil {
				logger.Error(err, "failed to modify owning object")
				return ctrl.Result{}, fmt.Errorf("modifying owning object: %w", err)
			}
			objUpdated = true
		}
	}

	if objUpdated {
		logger.Info("updating owning object ")
		if err := s.client.Update(ctx, obj); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("owning object was updated by another process, retrying")
				return ctrl.Result{Requeue: true}, nil
			}

			err = fmt.Errorf("failed to update owning object: %w", err)
			logger.Error(err, "failed to update owning object")
			s.events.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateUpstreamCertRef", err.Error())
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (s *secretProviderClassReconciler[objectType]) cleanupSpcOpt(ctx context.Context, lgr logr.Logger, opt spcOpts) error {
	if opt.action != actionCleanup {
		return errors.New("cleanupSpcOpt called with non-cleanup action")
	}

	lgr.Info("cleaning up SecretProviderClass")

	lgr.Info("getting SecretProviderClass to clean")
	toCleanSPC := &secv1.SecretProviderClass{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: opt.namespace, Name: opt.name}, toCleanSPC); err != nil {
		if client.IgnoreNotFound(err) != nil {
			lgr.Error(err, "failed to get SecretProviderClass to clean")
			return fmt.Errorf("getting SecretProviderClass to clean: %w", err)
		}

		lgr.Info("SecretProviderClass not found, nothing to clean")
		return nil
	}

	if manifests.HasTopLevelLabels(toCleanSPC.Labels) {
		lgr.Info("deleting SecretProviderClass")
		if err := s.client.Delete(ctx, toCleanSPC); err != nil {
			lgr.Error(err, "failed to delete SecretProviderClass")
			return fmt.Errorf("failed to delete SecretProviderClass: %w", err)
		}

		return nil
	}

	lgr.Info("SecretProviderClass does not have top-level labels, not managed by us")
	return nil
}

func (s *secretProviderClassReconciler[objectType]) buildSpc(obj client.Object, opts spcOpts) (*secv1.SecretProviderClass, error) {
	p := map[string]interface{}{
		"objectName": opts.certName,
		"objectType": "secret",
	}
	if opts.objectVersion != "" {
		p["objectVersion"] = opts.objectVersion
	}

	params, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshalling parameters: %w", err)
	}

	objects, err := json.Marshal(map[string]interface{}{"array": []string{string(params)}})
	if err != nil {
		return nil, fmt.Errorf("marshalling objects: %w", err)
	}

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.name,
			Namespace: opts.namespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				// todo: verify that the api version here is okay
				APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
				Controller: util.ToPtr(true),
				Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
				Name:       obj.GetName(),
				UID:        obj.GetUID(),
			}},
		},
		Spec: secv1.SecretProviderClassSpec{
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
		},
	}

	if opts.cloud != "" {
		spc.Spec.Parameters["cloud"] = opts.cloud
	}

	return spc, nil
}
