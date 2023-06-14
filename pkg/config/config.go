// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	// DefaultNs is the default namespace for the resources deployed by this operator
	DefaultNs       = "app-routing-system"
	PublicZoneType  = "dnszones"
	PrivateZoneType = "privatednszones"
)

var Flags = &Config{}
var dnsZonesString string

func init() {
	flag.StringVar(&Flags.NS, "namespace", DefaultNs, "namespace for managed resources")
	flag.StringVar(&Flags.Registry, "registry", "mcr.microsoft.com", "container image registry to use for managed components")
	flag.StringVar(&Flags.MSIClientID, "msi", "", "client ID of the MSI to use when accessing Azure resources")
	flag.StringVar(&Flags.TenantID, "tenant-id", "", "AAD tenant ID to use when accessing Azure resources")
	flag.StringVar(&Flags.Cloud, "cloud", "AzurePublicCloud", "azure cloud name")
	flag.StringVar(&Flags.Location, "location", "", "azure region name")
	flag.StringVar(&dnsZonesString, "dns-zone-ids", "", "dns zone resource IDs")
	flag.BoolVar(&Flags.DisableKeyvault, "disable-keyvault", false, "disable the keyvault integration")
	flag.Float64Var(&Flags.ConcurrencyWatchdogThres, "concurrency-watchdog-threshold", 200, "percentage of concurrent connections above mean required to vote for load shedding")
	flag.IntVar(&Flags.ConcurrencyWatchdogVotes, "concurrency-watchdog-votes", 4, "number of votes required for a pod to be considered for load shedding")
	flag.BoolVar(&Flags.DisableOSM, "disable-osm", false, "enable Open Service Mesh integration")
	flag.StringVar(&Flags.ServiceAccountTokenPath, "service-account-token-path", "", "optionally override the default token path")
	flag.StringVar(&Flags.MetricsAddr, "metrics-addr", "0.0.0.0:8081", "address to serve Prometheus metrics on")
	flag.StringVar(&Flags.ProbeAddr, "probe-addr", "0.0.0.0:8080", "address to serve readiness/liveness probes on")
	flag.StringVar(&Flags.OperatorDeployment, "operator-deployment", "app-routing-operator", "name of the operator's k8s deployment")
	flag.StringVar(&Flags.ClusterFqdn, "cluster-fqdn", "", "fqdn of the cluster the add-on belongs to")
}

type DnsZoneConfig struct {
	Subscription  string
	ResourceGroup string
	ZoneIds       []string
}

type Config struct {
	ServiceAccountTokenPath             string
	MetricsAddr, ProbeAddr              string
	NS, Registry                        string
	DisableKeyvault                     bool
	MSIClientID, TenantID               string
	Cloud, Location                     string
	PrivateZoneConfig, PublicZoneConfig *DnsZoneConfig
	ConcurrencyWatchdogThres            float64
	ConcurrencyWatchdogVotes            int
	DisableOSM                          bool
	OperatorDeployment                  string
	ClusterFqdn                         string
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
	if c.ConcurrencyWatchdogThres <= 100 {
		return errors.New("--concurrency-watchdog-threshold must be greater than 100")
	}
	if c.ConcurrencyWatchdogVotes < 1 {
		return errors.New("--concurrency-watchdog-votes must be a positive number")
	}

	if c.ClusterFqdn == "" {
		return errors.New("--cluster-fqdn is required")
	} else {
		c.ClusterFqdn = hash(c.ClusterFqdn)
	}

	if dnsZonesString != "" {
		if err := c.ParseZoneIDs(dnsZonesString); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) ParseZoneIDs(zonesString string) error {

	c.PrivateZoneConfig = &DnsZoneConfig{}
	c.PublicZoneConfig = &DnsZoneConfig{}

	DNSZoneIDs := strings.Split(zonesString, ",")
	for _, zoneId := range DNSZoneIDs {
		if zoneId == "" {
			return errors.New("--dns-zone-ids must not contain empty strings")
		}

		parsedZone, err := azure.ParseResourceID(zoneId)
		if err != nil {
			return fmt.Errorf("error while parsing dns zone resource ID %s: %s", zoneId, err)
		}

		if !strings.EqualFold(parsedZone.Provider, "Microsoft.Network") {
			return fmt.Errorf("invalid resource provider %s from zone %s: resource ID must be a public or private DNS Zone resource ID from provider Microsoft.Network", parsedZone.Provider, zoneId)
		}

		if strings.EqualFold(parsedZone.ResourceType, PrivateZoneType) {
			// it's a private zone
			c.PrivateZoneConfig.ZoneIds = append(c.PrivateZoneConfig.ZoneIds, zoneId)
			c.PrivateZoneConfig.Subscription = parsedZone.SubscriptionID
			c.PrivateZoneConfig.ResourceGroup = parsedZone.ResourceGroup
		} else if strings.EqualFold(parsedZone.ResourceType, PublicZoneType) {
			// it's a public zone
			c.PublicZoneConfig.ZoneIds = append(c.PublicZoneConfig.ZoneIds, zoneId)
			c.PublicZoneConfig.Subscription = parsedZone.SubscriptionID
			c.PublicZoneConfig.ResourceGroup = parsedZone.ResourceGroup
		} else {
			return fmt.Errorf("error while parsing dns zone resource ID %s: detected invalid resource type %s", zoneId, parsedZone.ResourceType)
		}
	}

	return nil
}

func hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
