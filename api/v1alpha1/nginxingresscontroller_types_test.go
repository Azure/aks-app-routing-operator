package v1alpha1

import (
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validNginxIngressController() NginxIngressController {
	return NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "name",
		},
		Spec: NginxIngressControllerSpec{
			IngressClassName:     "ingressClassName",
			ControllerNamePrefix: "controllerNamePrefix",
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
				nic.Spec.ControllerNamePrefix = "-controllerNamePrefix"
				return nic
			}(),
			want: "spec.controllerNamePrefix must start with alphanumeric character",
		},
		{
			name: "controller name prefix contains invalid characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.ControllerNamePrefix = "controllerNamePrefix!"
				return nic
			}(),
			want: "spec.controllerNamePrefix must contain only alphanumeric characters, dashes, and periods",
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
			name: "missing ingress class name",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = ""
				return nic
			}(),
			want: "spec.ingressClassName must be specified",
		},
		{
			name: "ingress class name starts with non alphanum",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "-ingressClassName"
				return nic
			}(),
			want: "spec.ingressClassName must start with alphanumeric character",
		},
		{
			name: "ingress class name contains invalid characters",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingressClassName!"
				return nic
			}(),
			want: "spec.ingressClassName must contain only alphanumeric characters, dashes, and periods",
		},
		{
			name: "ingress class name ends with non alphanum",
			nic: func() NginxIngressController {
				nic := validNginxIngressController()
				nic.Spec.IngressClassName = "ingressClassName-"
				return nic
			}(),
			want: "spec.ingressClassName must end with alphanumeric character",
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
