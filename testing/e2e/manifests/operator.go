package manifests

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	operatorNs        = "kube-system"
	ManagedResourceNs = "app-routing-system"
)

var (
	operatorDeploymentLabels = map[string]string{
		"app": "app-routing-operator",
	}

	// AllUsedOperatorVersions is a list of all the operator versions used today
	AllUsedOperatorVersions = []OperatorVersion{OperatorVersion0_2_5, OperatorVersionLatest}

	// AllDnsZoneCounts is a list of all the dns zone counts
	AllDnsZoneCounts     = []DnsZoneCount{DnsZoneCountNone, DnsZoneCountOne, DnsZoneCountMultiple}
	NonZeroDnsZoneCounts = []DnsZoneCount{DnsZoneCountOne, DnsZoneCountMultiple}

	SingleStackIPFamilyPolicy = corev1.IPFamilyPolicySingleStack
)

// GenerateSelfSignedCert generates a self-signed TLS certificate and private key for testing
func GenerateSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Organization"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost", "*.example.com", "example.com"},
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	return certPEM, keyPEM, nil
}

var certPEM, keyPEM []byte

// Generate self-signed certificate for testing
func init() {
	var err error
	certPEM, keyPEM, err = GenerateSelfSignedCert()
	if err != nil {
		panic("failed to generate self-signed certificate: " + err.Error())
	}
}

func CreateDefaultDomainSecret(certPEM, keyPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-domain-cert",
			Namespace: operatorNs,
		},
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
}

// OperatorVersion is an enum for the different versions of the operator
type OperatorVersion uint

const (
	OperatorVersion0_2_1_Patch_7 OperatorVersion = iota // use iota to number with earlier versions being lower numbers
	OperatorVersion0_2_3_Patch_5
	OperatorVersion0_2_5

	// OperatorVersionLatest represents the latest version of the operator which is essentially whatever code changes this test is running against
	OperatorVersionLatest = math.MaxUint // this must always be the last/largest value in the enum because we order by value
)

func (o OperatorVersion) String() string {
	switch o {
	case OperatorVersion0_2_1_Patch_7:
		return "0.2.1-patch-7"
	case OperatorVersion0_2_3_Patch_5:
		return "0.2.3-patch-5"
	case OperatorVersion0_2_5:
		return "0.2.5"
	case OperatorVersionLatest:
		return "latest"
	default:
		return "unknown"
	}
}

// DnsZoneCount is enum for the number of dns zones but shouldn't be used directly. Use the exported fields of this type instead.
type DnsZoneCount uint

const (
	// DnsZoneCountNone represents no dns zones
	DnsZoneCountNone DnsZoneCount = iota
	// DnsZoneCountOne represents one dns zone
	DnsZoneCountOne
	// DnsZoneCountMultiple represents multiple dns zones
	DnsZoneCountMultiple
)

func (d DnsZoneCount) String() string {
	switch d {
	case DnsZoneCountNone:
		return "none"
	case DnsZoneCountOne:
		return "one"
	case DnsZoneCountMultiple:
		return "multiple"
	default:
		return "unknown"
	}
}

type DnsZones struct {
	Public  DnsZoneCount
	Private DnsZoneCount
}

type OperatorConfig struct {
	Version    OperatorVersion
	Msi        string
	TenantId   string
	Location   string
	Zones      DnsZones
	DisableOsm bool
}

func (o *OperatorConfig) image(latestImage string) string {
	switch o.Version {
	case OperatorVersion0_2_1_Patch_7:
		return "mcr.microsoft.com/aks/aks-app-routing-operator:0.2.1-patch-7"
	case OperatorVersion0_2_3_Patch_5:
		return "mcr.microsoft.com/aks/aks-app-routing-operator:0.2.3-patch-5"
	case OperatorVersion0_2_5:
		return "mcr.microsoft.com/aks/aks-app-routing-operator:0.2.5"
	case OperatorVersionLatest:
		return latestImage
	default:
		panic("unknown operator version")
	}
}

// args returns the arguments to pass to the operator
func (o *OperatorConfig) args(publicZones, privateZones []string) []string {
	if len(publicZones) < 2 || len(privateZones) < 2 {
		panic("not enough zones provided")
	}

	ret := []string{
		"--msi", o.Msi,
		"--tenant-id", o.TenantId,
		"--location", o.Location,
		"--namespace", ManagedResourceNs,
		"--cluster-uid", "test-cluster-uid",
	}

	if o.Version >= OperatorVersion0_2_5 {
		ret = append(ret, "--dns-sync-interval", (time.Second * 3).String())
		ret = append(ret, "--enable-gateway")
		ret = append(ret, "--disable-expensive-cache")
	}

	if o.Version >= OperatorVersionLatest {
		ret = append(ret, "--enable-workload-identity")
		ret = append(ret, "--enable-default-domain")
		ret = append(ret, "--default-domain-server-address", "http://default-domain-server.kube-system.svc.cluster.local:8080")

		// these two don't do anything yet in the e2e test but are needed so the operator can run
		ret = append(ret, "--default-domain-client-id", "test-default-domain-client-id")
		ret = append(ret, "--default-domain-zone-id", "/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.Network/dnszones/test-domain.com")
	}

	var zones []string
	switch o.Zones.Public {
	case DnsZoneCountMultiple:
		zones = append(zones, publicZones...)
	case DnsZoneCountOne:
		zones = append(zones, publicZones[0])
	}
	switch o.Zones.Private {
	case DnsZoneCountMultiple:
		zones = append(zones, privateZones...)
	case DnsZoneCountOne:
		zones = append(zones, privateZones[0])
	}
	if len(zones) > 0 {
		ret = append(ret, "--dns-zone-ids", strings.Join(zones, ","))
	}

	if o.DisableOsm {
		ret = append(ret, "--disable-osm")
	}

	return ret
}

func Operator(latestImage string, publicZones, privateZones []string, cfg *OperatorConfig, cleanDeploy bool) []client.Object {
	var ret []client.Object

	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ManagedResourceNs,
		},
	}

	serviceAccount := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
		},
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "app-routing-operator",
				Namespace: operatorNs,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "",
		},
	}

	baseDeployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: operatorNs,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: operatorDeploymentLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: operatorDeploymentLabels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "app-routing-operator",
					Containers: []corev1.Container{
						{
							Name:  "operator",
							Image: cfg.image(latestImage),
							Args:  cfg.args(publicZones, privateZones),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.IntOrString{
											IntVal: 8080,
											Type:   intstr.Int,
										},
										HTTPHeaders: nil,
									},
								},
								PeriodSeconds: 5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.IntOrString{
											IntVal: 8080,
											Type:   intstr.Int,
										},
										HTTPHeaders: nil,
									},
								},
								PeriodSeconds: 5,
							},
						},
					},
				},
			},
		},
	}

	podDisrutptionBudget := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy/v1",
			Kind:       "PodDisruptionBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-routing-operator",
			Namespace: "app-routing-system",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: to.Ptr(intstr.FromInt(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: operatorDeploymentLabels,
			},
		},
	}

	ret = append(ret, []client.Object{
		namespace,
		serviceAccount,
		clusterRoleBinding,
		podDisrutptionBudget,
	}...)

	if cfg.Version == OperatorVersionLatest {
		defaultDomainSecret := CreateDefaultDomainSecret(certPEM, keyPEM)
		ret = append(ret, defaultDomainSecret)
		ret = append(ret, DefaultDomainServer(operatorNs, "default-domain-server")...)
	}

	// edit and select relevant manifest config by version
	switch cfg.Version {
	default:
		ret = append(ret, baseDeployment)

		if cleanDeploy {
			ret = append(ret, NewNginxIngressController("default", "webapprouting.kubernetes.azure.com"))
		}
	}

	for _, obj := range ret {
		setGroupKindVersion(obj)
	}

	return ret
}
