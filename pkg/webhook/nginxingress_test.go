package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var scheme = runtime.NewScheme()

func init() {
	approutingv1alpha1.AddToScheme(scheme)
	clientgoscheme.AddToScheme(scheme)
}

var validNginxIngressController = &approutingv1alpha1.NginxIngressController{
	ObjectMeta: metav1.ObjectMeta{
		Name: "valid",
	},
	Spec: approutingv1alpha1.NginxIngressControllerSpec{
		IngressClassName:     "ingressclassname",
		ControllerNamePrefix: "prefix",
	},
}

var invalidNginxIngressController = &approutingv1alpha1.NginxIngressController{
	ObjectMeta: metav1.ObjectMeta{
		Name: "invalid",
	},
	Spec: approutingv1alpha1.NginxIngressControllerSpec{
		IngressClassName: "ingressclassname",
	},
}

func toRaw(n *approutingv1alpha1.NginxIngressController) []byte {
	raw, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(n)
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.Encode(raw)
	return buf.Bytes()
}

var validUser = func(_ context.Context, _ logr.Logger, _ client.Client, _ admission.Request) (string, error) {
	return "", nil
}

var invalidUser = func(_ context.Context, _ logr.Logger, _ client.Client, _ admission.Request) (string, error) {
	return "", errors.New("invalid user")
}

func TestNginxIngressResourceValidator(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	existingIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing",
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingIc))
	existingNic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "existing",
			ControllerNamePrefix: "prefix",
		},
	}
	require.NoError(t, cl.Create(context.Background(), existingNic))

	defaultIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: nginxingress.DefaultIcName,
		},
	}
	require.NoError(t, cl.Create(context.Background(), defaultIc))

	cases := []struct {
		name          string
		req           admission.Request
		authenticator authenticateFn
		expected      admission.Response
	}{
		{
			name: "valid nginx ingress controller, valid user",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(validNginxIngressController),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "invalid user",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
				},
			},
			authenticator: invalidUser,
			expected:      admission.Denied("invalid user"),
		},
		{
			name: "invalid nginx ingress controller, valid user",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(invalidNginxIngressController),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Denied("spec.controllerNamePrefix must be specified"),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Denied(fmt.Sprintf("IngressClass %s already exists. Delete or use a different spec.IngressClassName field", existingIc.Name)),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class, update",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class, delete",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class on other nginx ingress controller",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							copy.Name = "other"
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Denied(fmt.Sprintf("IngressClass %s already exists. Delete or use a different spec.IngressClassName field", existingNic.Spec.IngressClassName)),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class on other nginx ingress controller, updating",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							copy.Name = "other"
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "valid nginx ingress controller, valid user, existing ingress class on other nginx ingress controller, deleting",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = existingIc.Name
							copy.Name = "other"
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "valid nginx ingress controller, valid user, default nic",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = nginxingress.DefaultIcName
							copy.Name = nginxingress.DefaultNicName
							return copy
						}()),
					},
				},
			},
			authenticator: validUser,
			expected:      admission.Allowed(""),
		},
		{
			name: "valid nginx ingress controller, invalid user, default nic",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(func() *approutingv1alpha1.NginxIngressController {
							copy := validNginxIngressController.DeepCopy()
							copy.Spec.IngressClassName = nginxingress.DefaultIcName
							copy.Name = nginxingress.DefaultNicName
							return copy
						}()),
					},
				},
			},
			authenticator: invalidUser,
			expected:      admission.Denied("invalid user"),
		},
	}

	metrics.InitControllerMetrics(nginxResourceValidationName)
	beforeErrCount := testutils.GetErrMetricCount(t, nginxResourceValidationName)
	beforeSuccessCount := testutils.GetReconcileMetricCount(t, nginxResourceValidationName, metrics.LabelSuccess)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validator := nginxIngressResourceValidator{
				client:       cl,
				decoder:      admission.NewDecoder(cl.Scheme()),
				authenticate: tc.authenticator,
			}
			actual := validator.Handle(context.Background(), tc.req)

			if actual.Allowed != tc.expected.Allowed {
				t.Errorf("expected allowed %v, got %v", tc.expected.Allowed, actual.Allowed)
			}

			if tc.expected.Result != nil && tc.expected.Result.Message != actual.Result.Message {
				t.Errorf("expected message %v, got %v", tc.expected.Result.Message, actual.Result.Message)
			}
		})
	}

	require.Greater(t, testutils.GetErrMetricCount(t, nginxResourceValidationName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, nginxResourceValidationName, metrics.LabelSuccess), beforeSuccessCount)
}

func TestNginxIngressResourceMutator(t *testing.T) {
	cases := []struct {
		name     string
		req      admission.Request
		expected admission.Response
	}{
		{
			name: "no mutation, all fields set",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(&approutingv1alpha1.NginxIngressController{
							Spec: approutingv1alpha1.NginxIngressControllerSpec{
								IngressClassName:     "ingressClassName",
								ControllerNamePrefix: "prefix",
							},
						}),
					},
				},
			},
			expected: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
				},
			},
		},
		{
			name: "mutation",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: toRaw(&approutingv1alpha1.NginxIngressController{
							ObjectMeta: metav1.ObjectMeta{
								Name: "name",
							},
							Spec: approutingv1alpha1.NginxIngressControllerSpec{
								IngressClassName: "ingressClassName",
							},
						}),
					},
				},
			},
			expected: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: true,
				},
				Patches: []jsonpatch.JsonPatchOperation{
					{
						Operation: "add",
						Path:      "/spec/controllerNamePrefix",
						Value:     "nginx",
					},
				},
			},
		},
		{
			name: "mutation fails to decode bad input",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: []byte{0, 0, 1, 2, 3},
					},
				},
			},
			expected: admission.Response{
				AdmissionResponse: admissionv1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: fmt.Errorf("decoding NginxIngressController: %w", errors.New("failed decode")).Error(),
					},
				},
			},
		},
	}

	metrics.InitControllerMetrics(nginxResourceMutationName)
	beforeErrCount := testutils.GetErrMetricCount(t, nginxResourceMutationName)
	beforeSuccessCount := testutils.GetReconcileMetricCount(t, nginxResourceMutationName, metrics.LabelSuccess)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutator := nginxIngressResourceMutator{
				decoder: admission.NewDecoder(scheme),
			}
			actual := mutator.Handle(context.Background(), tc.req)

			if actual.Allowed != tc.expected.Allowed {
				t.Errorf("expected allowed %v, got %v", tc.expected.Allowed, actual.Allowed)
			}

			if len(actual.Patches) != len(tc.expected.Patches) {
				t.Errorf("expected %d patches, got %d", len(tc.expected.Patches), len(actual.Patches))
			}

			for i, patch := range actual.Patches {
				if !reflect.DeepEqual(patch, tc.expected.Patches[i]) {
					t.Errorf("expected patch %v, got %v", tc.expected.Patches[i], patch)
				}
			}
		})
	}

	require.Greater(t, testutils.GetErrMetricCount(t, nginxResourceMutationName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, nginxResourceMutationName, metrics.LabelSuccess), beforeSuccessCount)
}
