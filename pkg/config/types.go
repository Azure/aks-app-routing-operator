package config

import (
	"errors"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

// ControllerConfig specifies configuration options for an Ingress Controller
type ControllerConfig int

const (
	// Standard means no special configuration rules are applied to the Controller
	Standard ControllerConfig = iota
	// Public means the Ingress Controller is exposed externally and is public facing
	Public
	// Private means the Ingress Controller is internally facing and is backed by a private IP address
	Private
	// Off means the Ingress Controller isn't used
	Off
)

var controllerConfigMapping = map[ControllerConfig]string{
	Standard: "standard",
	Public:   "public",
	Private:  "private",
	Off:      "off",
}

func (c *ControllerConfig) String() string {
	if c == nil {
		return "nil"
	}

	if str, ok := controllerConfigMapping[*c]; ok {
		return str
	}

	return "unknown"
}

func (c *ControllerConfig) Set(val string) error {
	if val == "" {
		*c = Standard
		return nil
	}

	if controllerCfg, ok := util.ReverseMap(controllerConfigMapping)[val]; ok {
		*c = controllerCfg
		return nil
	}

	return errors.New("controller config value not recognized")
}

type DnsZoneConfig struct {
	Subscription  string
	ResourceGroup string
	ZoneIds       map[string]struct{}
}

type Config struct {
	DefaultController                   ControllerConfig
	ServiceAccountTokenPath             string
	MetricsAddr, ProbeAddr              string
	NS, Registry                        string
	DisableKeyvault                     bool
	MSIClientID, TenantID               string
	Cloud, Location                     string
	PrivateZoneConfig, PublicZoneConfig DnsZoneConfig
	ConcurrencyWatchdogThres            float64
	ConcurrencyWatchdogVotes            int
	DisableOSM                          bool
	OperatorDeployment                  string
	ClusterUid                          string
	DnsSyncInterval                     time.Duration
	CrdPath                             string
	EnableGateway                       bool
	DisableExpensiveCache               bool
	EnableManagedCertificates           bool
	EnableInternalLogging               bool
}
