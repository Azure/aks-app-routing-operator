package clients

import (
	"context"
	"fmt"
	"testing"
)

func Test_App_Please_Work(t *testing.T) {
	r, err := RandStringAlphaNum(5)
	mc := aks{
		name: fmt.Sprintf("davidgamerotest-%s", r),
	}
	mcName := mc.name
	app, err := NewApplication(context.Background(), fmt.Sprintf("%s-app", mcName))
	if err != nil {
		t.Fail()
		t.Errorf("creating app registration: %s", err.Error())
	}
	fmt.Println("created app successfully")

	// make a new service principal
	sp, err := NewServicePrincipal(context.Background(), fmt.Sprintf("%s-sp", mcName), app)
	if err != nil {
		t.Errorf("creating service principal: %s", err.Error())
		t.Fail()
	}

	fmt.Println("created service principal successfully")
	fmt.Println(sp)

	err = app.Delete(context.Background())

	if err != nil {
		t.Errorf("deleting app: %s", err.Error())
		t.Fail()
	}

	fmt.Println("deleted app successfully")
}
