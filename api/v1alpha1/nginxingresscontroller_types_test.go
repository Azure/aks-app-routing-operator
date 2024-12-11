package v1alpha1

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func validNginxIngressController() NginxIngressController {
	return NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "name",
		},
		Spec: NginxIngressControllerSpec{
			IngressClassName:     "ingressclassname.com",
			ControllerNamePrefix: "controller-name-prefix",
		},
	}
}

func TestNginxIngressControllerGetCondition(t *testing.T) {
	nic := validNginxIngressController()
	cond := metav1.Condition{
		Type:   "test",
		Status: metav1.ConditionTrue,
	}
	got := nic.GetCondition(cond.Type)
	if got != nil {
		t.Errorf("NginxIngressController.GetCondition() = %v, want nil", got)
	}

	meta.SetStatusCondition(&nic.Status.Conditions, cond)
	got = nic.GetCondition(cond.Type)
	if got == nil {
		t.Errorf("NginxIngressController.GetCondition() = nil, want %v", cond)
	}
	if got.Status != cond.Status {
		t.Errorf("NginxIngressController.GetCondition() = %v, want %v", got.Status, cond.Status)
	}
}

func TestNginxIngressControllerSetCondition(t *testing.T) {
	// new condition
	nic := validNginxIngressController()
	nic.Generation = 456

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	nic.SetCondition(cond)
	got := nic.GetCondition(cond.Type)
	if got == nil {
		t.Errorf("NginxIngressController.SetCondition() = nil, want %v", cond)
	}
	if got.Status != cond.Status {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Status, cond.Status)
	}
	if got.ObservedGeneration != nic.Generation {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.ObservedGeneration, nic.Generation)
	}
	if got.Reason != cond.Reason {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Reason, cond.Reason)
	}
	if got.Message != cond.Message {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Message, cond.Message)
	}

	// set same condition
	nic.Generation = 789
	nic.SetCondition(cond)
	got = nic.GetCondition(cond.Type)
	if got == nil {
		t.Errorf("NginxIngressController.SetCondition() = nil, want %v", cond)
	}
	if got.Status != cond.Status {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Status, cond.Status)
	}
	if got.ObservedGeneration != nic.Generation {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.ObservedGeneration, nic.Generation)
	}
	if got.Reason != cond.Reason {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Reason, cond.Reason)
	}
	if got.Message != cond.Message {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Message, cond.Message)
	}

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	nic.SetCondition(cond2)
	got = nic.GetCondition(cond2.Type)
	if got == nil {
		t.Errorf("NginxIngressController.SetCondition() = nil, want %v", cond2)
	}
	if got.Status != cond2.Status {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Status, cond2.Status)
	}
	if got.ObservedGeneration != nic.Generation {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.ObservedGeneration, nic.Generation)
	}
	if got.Reason != cond2.Reason {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Reason, cond2.Reason)
	}
	if got.Message != cond2.Message {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Message, cond2.Message)
	}

	// old condition should not be changed
	got = nic.GetCondition(cond.Type)
	if got == nil {
		t.Errorf("NginxIngressController.SetCondition() = nil, want %v", cond)
	}
	if got.Status != cond.Status {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Status, cond.Status)
	}
	if got.ObservedGeneration != nic.Generation {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.ObservedGeneration, nic.Generation)
	}
	if got.Reason != cond.Reason {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Reason, cond.Reason)
	}
	if got.Message != cond.Message {
		t.Errorf("NginxIngressController.SetCondition() = %v, want %v", got.Message, cond.Message)
	}
}

func TestVerifyAndSetCondition(t *testing.T) {
	// new condition
	nic := validNginxIngressController()
	nic.Generation = 456

	cond := metav1.Condition{
		Type:    "test",
		Status:  metav1.ConditionTrue,
		Reason:  "reason",
		Message: "message",
	}

	VerifyAndSetCondition(&nic, cond)
	got := nic.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set same condition
	nic.Generation = 789
	VerifyAndSetCondition(&nic, cond)
	got = nic.GetCondition(cond.Type)

	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)

	// set different condition
	cond2 := metav1.Condition{
		Type:   "test2",
		Status: metav1.ConditionTrue,
	}
	VerifyAndSetCondition(&nic, cond2)
	got = nic.GetCondition(cond2.Type)
	require.NotNil(t, got)
	require.Equal(t, cond2.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond2.Reason, got.Reason)
	require.Equal(t, cond2.Message, got.Message)

	// old condition should not be changed
	got = nic.GetCondition(cond.Type)
	require.NotNil(t, got)
	require.Equal(t, cond.Status, got.Status)
	require.Equal(t, nic.Generation, got.ObservedGeneration)
	require.Equal(t, cond.Reason, got.Reason)
	require.Equal(t, cond.Message, got.Message)
}

func TestNginxIngressControllerCollides(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	existingIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing",
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingIc))
	existingNic := &NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing2",
		},
		Spec: NginxIngressControllerSpec{
			IngressClassName:     "existing2",
			ControllerNamePrefix: "prefix3",
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingNic))
	existingNicWithExistingIc := &NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing3",
		},
		Spec: NginxIngressControllerSpec{
			IngressClassName:     "existing3",
			ControllerNamePrefix: "prefix3",
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingNicWithExistingIc))
	existingIcWithExistingNic := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: existingNicWithExistingIc.Spec.IngressClassName,
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingIcWithExistingNic))

	cases := []struct {
		name          string
		reqNic        *NginxIngressController
		wantCollision bool
		wantReason    string
		wantErr       error
	}{
		{
			name: "no collision",
			reqNic: &NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new",
				},
				Spec: NginxIngressControllerSpec{
					IngressClassName: "new",
				},
			},
		},
		{
			name: "collision with existing IngressClass",
			reqNic: &NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new",
				},
				Spec: NginxIngressControllerSpec{
					IngressClassName: existingIc.Name,
				},
			},
			wantCollision: true,
			wantReason:    "spec.ingressClassName \"existing\" is invalid because IngressClass \"existing\" already exists",
		},
		{
			name: "collision with existing NginxIngressController",
			reqNic: &NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new",
				},
				Spec: NginxIngressControllerSpec{
					IngressClassName: existingNic.Spec.IngressClassName,
				},
			},
			wantCollision: true,
			wantReason:    "spec.ingressClassName \"existing2\" is invalid because NginxIngressController \"existing2\" already uses IngressClass \"existing2\"",
		},
		{
			name: "collision with existing NginxIngressController and IngressClass should show NginxIngressController reason",
			reqNic: &NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new",
				},
				Spec: NginxIngressControllerSpec{
					IngressClassName: existingNicWithExistingIc.Spec.IngressClassName,
				},
			},
			wantCollision: true,
			wantReason:    "spec.ingressClassName \"existing3\" is invalid because NginxIngressController \"existing3\" already uses IngressClass \"existing3\"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotCollision, gotReason, gotErr := c.reqNic.Collides(context.Background(), cl)
			if gotCollision != c.wantCollision {
				t.Errorf("NginxIngressController.Collides() gotCollision = %v, want %v", gotCollision, c.wantCollision)
			}
			if gotReason != c.wantReason {
				t.Errorf("NginxIngressController.Collides() gotReason = %v, want %v", gotReason, c.wantReason)
			}
			if gotErr != c.wantErr {
				t.Errorf("NginxIngressController.Collides() gotErr = %v, want %v", gotErr, c.wantErr)
			}
		})
	}

	t.Run("client errors", func(t *testing.T) {
		listErr := errors.New("list error")
		listErrCl := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return listErr
			},
		}).WithScheme(scheme).Build()
		getErr := errors.New("get error")
		getErrCl := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return getErr
			},
		}).WithScheme(scheme).Build()

		collision, reason, err := existingNic.Collides(context.Background(), listErrCl)
		require.False(t, collision)
		require.Empty(t, reason)
		require.True(t, errors.Is(err, listErr), "expected error \"%v\", to be type \"%v\"", err, listErr)

		collision, reason, err = existingNic.Collides(context.Background(), getErrCl)
		require.False(t, collision)
		require.Empty(t, reason)
		require.True(t, errors.Is(err, getErr), "expected error \"%v\", to be type \"%v\"", err, getErr)
	})
}
