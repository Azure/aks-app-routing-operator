// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest/azure"
	flag "github.com/spf13/pflag"
)

const (
	// DefaultNs is the default namespace for the resources deployed by this operator
	DefaultNs       = "app-routing-system"
	PublicZoneType  = "dnszones"
	PrivateZoneType = "privatednszones"
)

var Flags = &Config{}

func init() {
	flag.StringVar(&Flags.NS, "namespace", DefaultNs, "namespace for managed resources")
	flag.StringVar(&Flags.Registry, "registry", "mcr.microsoft.com", "container image registry to use for managed components")
	flag.StringVar(&Flags.MSIClientID, "msi", "", "client ID of the MSI to use when accessing Azure resources")
	flag.StringVar(&Flags.TenantID, "tenant-id", "", "AAD tenant ID to use when accessing Azure resources")
	flag.StringVar(&Flags.Cloud, "cloud", "AzurePublicCloud", "azure cloud name")
	flag.StringVar(&Flags.Location, "location", "", "azure region name")
	flag.StringVar(&Flags.DNSZoneSub, "dns-zone-subscription", "", "subscription ID of the Azure DNS Zone (optional)")
	Flags.DNSZoneIDs = flag.StringSlice("dns-zone-ids", []string{}, "comma-separated strings of DNS Zones associated with the add-on (optional)")
	flag.BoolVar(&Flags.DisableKeyvault, "disable-keyvault", false, "disable the keyvault integration")
	flag.Float64Var(&Flags.ConcurrencyWatchdogThres, "concurrency-watchdog-threshold", 200, "percentage of concurrent connections above mean required to vote for load shedding")
	flag.IntVar(&Flags.ConcurrencyWatchdogVotes, "concurrency-watchdog-votes", 4, "number of votes required for a pod to be considered for load shedding")
	flag.BoolVar(&Flags.DisableOSM, "disable-osm", false, "enable Open Service Mesh integration")
	flag.StringVar(&Flags.ServiceAccountTokenPath, "service-account-token-path", "", "optionally override the default token path")
	flag.StringVar(&Flags.MetricsAddr, "metrics-addr", "0.0.0.0:8081", "address to serve Prometheus metrics on")
	flag.StringVar(&Flags.ProbeAddr, "probe-addr", "0.0.0.0:8080", "address to serve readiness/liveness probes on")
	flag.StringVar(&Flags.OperatorDeployment, "operator-deployment", "app-routing-operator", "name of the operator's k8s deployment")
}

type Config struct {
	ServiceAccountTokenPath  string
	MetricsAddr, ProbeAddr   string
	NS, Registry             string
	DisableKeyvault          bool
	MSIClientID, TenantID    string
	Cloud, Location          string
	DNSZoneSub               string
	DNSZoneIDs               *[]string
	DNSZoneDomains           []string
	ConcurrencyWatchdogThres float64
	ConcurrencyWatchdogVotes int
	DisableOSM               bool
	OperatorDeployment       string
}

func (c *Config) Validate() error {
	if c.NS == "" {
		return errors.New("--namespace is required")
	}
	if c.Registry == "" {
		return errors.New("--registry is required")
	}
	if c.MSIClientID == "" {
		return errors.New("--msi is required")
	}
	if c.TenantID == "" {
		return errors.New("--tenant-id is required")
	}
	if c.Cloud == "" {
		return errors.New("--cloud is required")
	}
	if c.Location == "" {
		return errors.New("--location is required")
	}
	if c.DNSZoneSub != "" {
		if c.DNSZoneSub == "" {
			return errors.New("--dns-zone-subscription is required")
		}
	}
	if c.ConcurrencyWatchdogThres <= 100 {
		return errors.New("--concurrency-watchdog-threshold must be greater than 100")
	}
	if c.ConcurrencyWatchdogVotes < 1 {
		return errors.New("--concurrency-watchdog-votes must be a positive number")
	}

	c.DNSZoneDomains = make([]string, len(*c.DNSZoneIDs))
	for _, id := range *c.DNSZoneIDs {
		if id == "" {
			return errors.New("--dns-zone-ids must not contain empty strings")
		}

		parsedID, err := azure.ParseResourceID(id)
		if err != nil {
			return fmt.Errorf("failed to parse DNS Zone ID %s: %s", id, err)
		}

		isPublicDnsZone := strings.EqualFold(parsedID.ResourceType, PublicZoneType)
		isPrivateDnsZone := strings.EqualFold(parsedID.ResourceType, PrivateZoneType)

		if !strings.EqualFold(parsedID.Provider, "Microsoft.Network") || (!isPublicDnsZone && !isPrivateDnsZone) {
			return fmt.Errorf("invalid DNS Zone ID %s: must be a public or private DNS Zone resource ID", id)
		}
		c.DNSZoneDomains = append(c.DNSZoneDomains, parsedID.ResourceName)
	}

	return nil
}
