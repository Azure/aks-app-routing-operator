package dns

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type CleanupRunner struct {
	client           client.Client
	unusedNameLabels []string
}

var _ manager.Runnable = &CleanupRunner{}

func newCleanupRunner(manager ctrl.Manager, namesToDelete []string) error {
	runner := &CleanupRunner{
		client:           manager.GetClient(),
		unusedNameLabels: namesToDelete,
	}
	return manager.Add(runner)
}

func (r *CleanupRunner) Start(ctx context.Context) error {
	// deletion would take place here using manifests.K8sNameKey

	return nil
}
