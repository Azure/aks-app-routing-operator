package v1alpha1

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestNginxIngressControllerValid(t *testing.T) {
	cases := []struct {
		name string
		nic  NginxIngressController
		want string
	}{
		{
			name: "valid NginxIngressController",
			nic:  validNginxIngressController(),
			want: "",
		},
		{
			name: "missing controller name prefix",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = ""
				return nic
			}(),
			want: "spec.controllerNamePrefix must be specified",
		},
		{
			name: "controller name prefix starts with non alphanum",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = "-controllernameprefix"
				return nic
			}(),
			want: "spec.controllerNamePrefix must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "controller name prefix contains invalid characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = "controllernameprefix!"
				return nic
			}(),
			want: "spec.controllerNamePrefix must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "controller name prefix too long",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = strings.Repeat("a", maxControllerNamePrefix+1)
				return nic
			}(),
			want: fmt.Sprintf("spec.controllerNamePrefix length must be less than or equal to %d characters", maxControllerNamePrefix),
		},
		{
			name: "controller name prefix capitalized",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = "ControllerNamePrefix"
				return nic
			}(),
			want: "spec.controllerNamePrefix must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "missing ingress class name",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = ""
				return nic
			}(),
			want: "spec.ingressClassName must be specified",
		},
		{
			name: "ingress class name capitalized",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "IngressClassName"
				return nic
			}(),
			want: "spec.ingressClassName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "ingress class name capitalized with special characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingress-Class.Name"
				return nic
			}(),
			want: "spec.ingressClassName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "ingress class name with special characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingress-class.name"
				return nic
			}(),
			want: "",
		},
		{
			name: "ingress class name starts with non alphanum",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "-ingressclassname"
				return nic
			}(),
			want: "spec.ingressClassName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "ingress class name contains invalid characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingressclassname!"
				return nic
			}(),
			want: "spec.ingressClassName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "ingress class name ends with non alphanum",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingressclassname-"
				return nic
			}(),
			want: "spec.ingressClassName must be a lowercase RFC 1123 subdomain consisting of lowercase alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character",
		},
		{
			name: "long name",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.ObjectMeta.Name = strings.Repeat("a", maxNameLength+1)
				return nic
			}(),
			want: fmt.Sprintf("Name length must be less than or equal to %d characters", maxNameLength),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.nic.Valid()
			if got != c.want {
				t.Errorf("NginxIngressController.Valid() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNginxIngressControllerDefault(t *testing.T) {
	t.Run("default ingress class name", func(t *testing.T) {
		nic := validNginxIngressController()
		nic.Spec.IngressClassName = ""
		nic.Default()
		expected := nic.Name
		if nic.Spec.IngressClassName != expected {
			t.Errorf("NginxIngressController.Default() = %v, want %v", nic.Spec.IngressClassName, expected)
		}
	})

	t.Run("default controller name prefix", func(t *testing.T) {
		nic := validNginxIngressController()
		nic.Spec.ControllerNamePrefix = ""
		nic.Default()
		expected := defaultControllerNamePrefix
		if nic.Spec.ControllerNamePrefix != expected {
			t.Errorf("NginxIngressController.Default() = %v, want %v", nic.Spec.ControllerNamePrefix, expected)
		}
	})

	t.Run("doesn't overwrite ingress class name", func(t *testing.T) {
		nic := validNginxIngressController()
		existingIngressClassName := "existingIngressClassName"
		nic.Spec.IngressClassName = existingIngressClassName
		nic.Default()
		if nic.Spec.IngressClassName != existingIngressClassName {
			t.Errorf("NginxIngressController.Default() = %v, want %v", nic.Spec.IngressClassName, existingIngressClassName)
		}
	})

	t.Run("doesn't overwrite controller name prefix", func(t *testing.T) {
		nic := validNginxIngressController()
		existingControllerNamePrefix := "existingControllerNamePrefix"
		nic.Spec.ControllerNamePrefix = existingControllerNamePrefix
		nic.Default()
		if nic.Spec.ControllerNamePrefix != existingControllerNamePrefix {
			t.Errorf("NginxIngressController.Default() = %v, want %v", nic.Spec.ControllerNamePrefix, existingControllerNamePrefix)
		}
	})
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

func TestStartsWithAlphaNum(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "starts with alpha",
			s:    "a",
			want: true,
		},
		{
			name: "starts with num",
			s:    "1",
			want: true,
		},
		{
			name: "empty",
			s:    "",
			want: false,
		},
		{
			name: "longer starts with alpha",
			s:    "abc23",
			want: true,
		},
		{
			name: "longer starts with num",
			s:    "123abc",
			want: true,
		},
		{
			name: "starts with dash",
			s:    "-abc",
			want: false,
		},
		{
			name: "starts with period",
			s:    ".123",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := startsWithAlphaNum(c.s)
			if got != c.want {
				t.Errorf("startsWithAlphaNum(%v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}

func TestEndsWithAlphaNum(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "ends with alpha",
			s:    "a",
			want: true,
		},
		{
			name: "ends with num",
			s:    "1",
			want: true,
		},
		{
			name: "empty",
			s:    "",
			want: false,
		},
		{
			name: "longer ends with alpha",
			s:    "abc23b",
			want: true,
		},
		{
			name: "longer ends with num",
			s:    "123abc22",
			want: true,
		},
		{
			name: "ends with dash",
			s:    "abc-",
			want: false,
		},
		{
			name: "ends with period",
			s:    "123.",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := endsWithAlphaNum(c.s)
			if got != c.want {
				t.Errorf("endsWithAlphaNum(%v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}

func TestOnlyAlphaNumDashPeriod(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "only alpha",
			s:    "abc",
			want: true,
		},
		{
			name: "only num",
			s:    "123",
			want: true,
		},
		{
			name: "only dash",
			s:    "---",
			want: true,
		},
		{
			name: "only period",
			s:    "...",
			want: true,
		},
		{
			name: "alpha num dash period",
			s:    "abc123.-",
			want: true,
		},
		{
			name: "empty",
			s:    "",
			want: true,
		},
		{
			name: "alpha num dash period with space",
			s:    "abc 123.-",
			want: false,
		},
		{
			name: "alpha num dash period with underscore",
			s:    "abc_123.-",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := onlyAlphaNumDashPeriod(c.s)
			if got != c.want {
				t.Errorf("onlyAlphaNumDashPeriod(%v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}

func TestIsLower(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "lower",
			s:    "abc",
			want: true,
		},
		{
			name: "upper",
			s:    "ABC",
			want: false,
		},
		{
			name: "mixed",
			s:    "AbC",
			want: false,
		},
		{
			name: "empty",
			s:    "",
			want: true,
		},
		{
			name: "lower with space",
			s:    "abc ",
			want: true,
		},
		{
			name: "lower with underscore",
			s:    "abc_",
			want: true,
		},
		{
			name: "lower with dash",
			s:    "abc-",
			want: true,
		},
		{
			name: "lower with period",
			s:    "abc.",
			want: true,
		},
		{
			name: "upper with space",
			s:    "ABC ",
			want: false,
		},
		{
			name: "upper with underscore",
			s:    "ABC_",
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isLower(c.s)
			if got != c.want {
				t.Errorf("isLower(%v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}
