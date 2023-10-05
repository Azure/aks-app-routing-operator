package clients

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/utils/env"
)

func Test_App_Please_Work(t *testing.T) {
	applicationObjectId := env.GetString("SERVICE_PRINCIPAL_APP_OBJ_ID", "")
	if applicationObjectId == "" {
		t.Errorf("SERVICE_PRINCIPAL_APP_OBJ_ID env var not set")
		t.Fail()
	}

	n, err := RandStringAlphaNum(10)
	if err != nil {
		t.Errorf("generating random string: %s", err.Error())
		t.Fail()
	}
	credName := n + "-cred"

	spOpt, err := GetServicePrincipalOptions(context.TODO(), applicationObjectId, credName)
	if err != nil {
		t.Errorf("getting application: %s", err.Error())
		t.Fail()
	}
	fmt.Printf("app: %+v\n", spOpt)

}
