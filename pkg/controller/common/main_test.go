package common

import (
	"os"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	restConfig *rest.Config
	err        error
	env        *envtest.Environment
)

func TestMain(m *testing.M) {
	restConfig, env, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv(env)

	os.Exit(code)
}
