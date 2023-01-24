package informer

import (
	"context"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// https://groups.google.com/g/kubernetes-sig-api-machinery/c/PbSCXdLDno0 discussion on resync time
const informerResync = time.Hour * 24

// NewFactory constructs a new instance of sharedInformerFactory that starts with a manager
func NewFactory(m ctrl.Manager) (informers.SharedInformerFactory, error) {
	clientset, err := kubernetes.NewForConfig(m.GetConfig())
	if err != nil {
		return nil, err
	}

	factory := informers.NewSharedInformerFactory(clientset, informerResync)

	if err := m.Add(manager.RunnableFunc(func(ctx context.Context) error {
		m.GetLogger().WithName("informerFactory").Info("starting")
		factory.Start(ctx.Done())
		return nil
	})); err != nil {
		return nil, err
	}

	return factory, nil
}
