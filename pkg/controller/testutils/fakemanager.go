package testutils

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ ctrl.Manager = &FakeManager{}

type FakeManager struct {
	Client client.Client
	Scheme *runtime.Scheme
}

// Add implements manager.Manager.
func (f *FakeManager) Add(manager.Runnable) error {
	return nil
}

// AddHealthzCheck implements manager.Manager.
func (f *FakeManager) AddHealthzCheck(name string, check healthz.Checker) error {
	return nil
}

// AddMetricsExtraHandler implements manager.Manager.
func (f *FakeManager) AddMetricsExtraHandler(path string, handler http.Handler) error {
	return nil
}

// AddReadyzCheck implements manager.Manager.
func (f *FakeManager) AddReadyzCheck(name string, check healthz.Checker) error {
	return nil
}

// Elected implements manager.Manager.
func (f *FakeManager) Elected() <-chan struct{} {
	return nil
}

// GetAPIReader implements manager.Manager.
func (f *FakeManager) GetAPIReader() client.Reader {
	return nil
}

// GetCache implements manager.Manager.
func (f *FakeManager) GetCache() cache.Cache {
	return nil
}

// GetClient implements manager.Manager.
func (f *FakeManager) GetClient() client.Client {
	return f.Client
}

// GetConfig implements manager.Manager.
func (f *FakeManager) GetConfig() *rest.Config {
	return nil
}

// GetControllerOptions implements manager.Manager.
func (f *FakeManager) GetControllerOptions() v1alpha1.ControllerConfigurationSpec {
	return v1alpha1.ControllerConfigurationSpec{}
}

// GetEventRecorderFor implements manager.Manager.
func (f *FakeManager) GetEventRecorderFor(name string) record.EventRecorder {
	return nil
}

// GetFieldIndexer implements manager.Manager.
func (f *FakeManager) GetFieldIndexer() client.FieldIndexer {
	return nil
}

// GetLogger implements manager.Manager.
func (f *FakeManager) GetLogger() logr.Logger {
	return logr.Discard()
}

// GetRESTMapper implements manager.Manager.
func (f *FakeManager) GetRESTMapper() meta.RESTMapper {
	return nil
}

// GetScheme implements manager.Manager.
func (f *FakeManager) GetScheme() *runtime.Scheme {
	return f.Scheme
}

// GetWebhookServer implements manager.Manager.
func (f *FakeManager) GetWebhookServer() *webhook.Server {
	return nil
}

// SetFields implements manager.Manager.
func (f *FakeManager) SetFields(interface{}) error {
	return nil
}

// Start implements manager.Manager.
func (f *FakeManager) Start(ctx context.Context) error {
	return nil
}
