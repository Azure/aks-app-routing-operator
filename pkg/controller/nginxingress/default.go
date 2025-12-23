package nginxingress

import (
	"context"
	"errors"
	"fmt"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultcName is the default Ingress class name
	DefaultIcName = "webapprouting.kubernetes.azure.com"
	// DefaultNicName is the default Nginx Ingress Controller resource name
	DefaultNicName         = "default"
	DefaultNicResourceName = "nginx"

	reconcileInterval = time.Minute * 3
	retryInterval     = time.Second
)

const internalLbAnnotation = "service.beta.kubernetes.io/azure-load-balancer-internal"

func NewDefaultReconciler(mgr ctrl.Manager, conf *config.Config) error {
	if conf == nil {
		return errors.New("nil config")
	}

	if conf.DefaultController == config.Off {
		return nil
	}

	name := controllername.New("default", "nginx", "ingress", "controller", "reconciler")
	metrics.InitControllerMetrics(name)
	if err := mgr.Add(&defaultNicReconciler{
		name:   name,
		lgr:    name.AddToLogger(mgr.GetLogger()),
		client: mgr.GetClient(),
		conf:   *conf,
	}); err != nil {
		return fmt.Errorf("adding default nginx ingress controller: %w", err)
	}

	return nil
}

type defaultNicReconciler struct {
	name   controllername.ControllerNamer
	client client.Client
	lgr    logr.Logger
	conf   config.Config
}

func (d *defaultNicReconciler) Start(ctx context.Context) error {
	d.lgr.Info("starting default nginx ingress controller reconciler")
	interval := time.Nanosecond
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(util.Jitter(interval, 0.3)):
		}

		if err := d.tick(ctx); err != nil {
			d.lgr.Error(err, "reconciling default nginx ingress controller")
			interval = retryInterval
			continue
		}

		interval = reconcileInterval
	}
}

func (d *defaultNicReconciler) tick(ctx context.Context) (err error) {
	start := time.Now()
	d.lgr.Info("starting to reconcile default nginx ingress controller")
	defer func() {
		d.lgr.Info("finished reconciling default nginx ingress controller", "latencySec", time.Since(start).Seconds())
		metrics.HandleControllerReconcileMetrics(d.name, ctrl.Result{}, err)
	}()

	nic := GetDefaultNginxIngressController()
	switch d.conf.DefaultController {
	case config.Public:
		nic.Spec.LoadBalancerAnnotations = map[string]string{
			internalLbAnnotation: "false",
		}
	case config.Private:
		nic.Spec.LoadBalancerAnnotations = map[string]string{
			internalLbAnnotation: "true",
		}
	}

	d.lgr.Info("upserting default nginx ingress controller")
	if err := util.Upsert(ctx, d.client, &nic); err != nil {
		d.lgr.Error(err, "upserting default nginx ingress controller")
		return fmt.Errorf("upserting default nginx ingress controller: %w", err)
	}

	return nil
}

func GetDefaultNginxIngressController() approutingv1alpha1.NginxIngressController {
	return approutingv1alpha1.NginxIngressController{
		TypeMeta: metav1.TypeMeta{
			APIVersion: approutingv1alpha1.GroupVersion.String(),
			Kind:       "NginxIngressController",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicName,
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			ControllerNamePrefix: DefaultNicResourceName,
			IngressClassName:     DefaultIcName,
		},
	}
}

// GetDefaultIngressClassControllerClass returns the default ingress class controller class
func GetDefaultIngressClassControllerClass(cl client.Client) (string, error) {
	defaultNicCc := "webapprouting.kubernetes.azure.com/nginx"

	defaultIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultIcName,
		},
	}
	err := cl.Get(context.Background(), types.NamespacedName{Name: DefaultIcName}, defaultIc)
	if err == nil {
		defaultNicCc = defaultIc.Spec.Controller
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", fmt.Errorf("getting default ingress class: %w", err)
	}

	return defaultNicCc, nil
}

// IsDefaultNic returns true if the given NginxIngressController is the default one
func IsDefaultNic(nic *approutingv1alpha1.NginxIngressController) bool {
	if nic == nil {
		return false
	}

	return nic.Name == DefaultNicName && nic.Spec.IngressClassName == DefaultIcName
}
