package common

import (
	"os"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"k8s.io/client-go/rest"
)

var (
	restConfig *rest.Config
	err        error
)

func TestMain(m *testing.M) {
	restConfig, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv()

	os.Exit(code)
}
