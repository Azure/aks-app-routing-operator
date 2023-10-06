package webhook

import "sigs.k8s.io/controller-runtime/pkg/manager"

// AddToManagerFuncs is a list of functions to add all Webhooks to the Manager
var AddToManagerFns []func(manager.Manager) error

// AddToManager adds all Webhooks to the Manager
func AddToManager(m manager.Manager) error {
	for _, f := range AddToManagerFns {
		if err := f(m); err != nil {
			return err
		}
	}

	return nil

}
