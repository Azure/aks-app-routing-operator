// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	// DefaultNs is the default namespace for the resources deployed by this operator
	DefaultNs              = "app-routing-system"
	PublicZoneType         = "dnszones"
	PrivateZoneType        = "privatednszones"
	defaultDnsSyncInterval = 3 * time.Minute
)

var Flags = &Config{}
var dnsZonesString string

func init() {
	flag.BoolVar(&Flags.IsInitContainer, "init-container", false, "whether the image is running as an init container for setup for the operator")
	flag.StringVar(&Flags.CertSecretName, "cert-secret", "app-routing-webhook-secret", "name of the secret containing the webhook cert")
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
	flag.StringVar(&Flags.OperatorNs, "operator-namespace", "kube-system", "namespace of the operator's k8s deployment")
	flag.StringVar(&Flags.OperatorWebhookService, "operator-webhook-service", "app-routing-operator-webhook", "name of the operator's webhook service")
	flag.IntVar(&Flags.WebhookPort, "webhook-port", 9443, "port to serve the webhook on")
	flag.StringVar(&Flags.ClusterUid, "cluster-uid", "", "unique identifier of the cluster the add-on belongs to")
	flag.DurationVar(&Flags.DnsSyncInterval, "dns-sync-interval", defaultDnsSyncInterval, "interval at which to sync DNS records")
	flag.StringVar(&Flags.CrdPath, "crd", "/crd", "location of the CRD manifests. manifests should be directly in this directory, not in a subdirectory")
	flag.StringVar(&Flags.CertDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "location of the certificates")
	flag.StringVar(&Flags.CertName, "cert-name", "tls.crt", "name of the certificate file in the cert-dir")
	flag.StringVar(&Flags.KeyName, "key-name", "tls.key", "name of the key file in the cert-dir")
	flag.StringVar(&Flags.CaName, "ca-name", "ca.crt", "name of the CA file in the cert-dir")
}

type DnsZoneConfig struct {
	Subscription  string
	ResourceGroup string
	ZoneIds       []string
}

type Config struct {
	IsInitContainer                     bool
	CertSecretName                      string
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
	OperatorNs                          string
	OperatorDeployment                  string
	OperatorWebhookService              string
	WebhookPort                         int
	ClusterUid                          string
	DnsSyncInterval                     time.Duration
	CrdPath                             string
	CertDir                             string
	CertName, KeyName, CaName           string
}

func (c *Config) Validate() error {
	if c.OperatorNs == "" {
		return errors.New("--operator-namespace is required")
	}
	if c.OperatorWebhookService == "" {
		return errors.New("--operator-webhook-service is required")
	}
	if c.CertName == "" {
		return errors.New("--cert-name is required")
	}
	if c.KeyName == "" {
		return errors.New("--key-name is required")
	}
	if c.CaName == "" {
		return errors.New("--ca-name is required")
	}

	// placement of where we check if it's the init container is very important
	// not all flags are required for the init container so this exits early
	if c.IsInitContainer {
		if c.CertSecretName == "" {
			return errors.New("--cert-secret is required")
		}

		return nil
	}

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
	if c.WebhookPort == 0 {
		return errors.New("--webhook-port is required")
	}
	if c.OperatorDeployment == "" {
		return errors.New("--operator-deployment is required")
	}

	if c.ClusterUid == "" {
		return errors.New("--cluster-uid is required")
	}

	if dnsZonesString != "" {
		if err := c.ParseAndValidateZoneIDs(dnsZonesString); err != nil {
			return err
		}
	}

	if c.DnsSyncInterval <= 0 {
		c.DnsSyncInterval = defaultDnsSyncInterval
	}

	crdPathStat, err := os.Stat(c.CrdPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("crd path %s does not exist", c.CrdPath)
	}
	if err != nil {
		return fmt.Errorf("checking crd path %s: %s", c.CrdPath, err)
	}
	if !crdPathStat.IsDir() {
		return fmt.Errorf("crd path %s is not a directory", c.CrdPath)
	}

	if c.CertDir == "" {
		return errors.New("--cert-dir is required")
	}

	return nil
}

func (c *Config) ParseAndValidateZoneIDs(zonesString string) error {

	c.PrivateZoneConfig = DnsZoneConfig{}
	c.PublicZoneConfig = DnsZoneConfig{}

	DNSZoneIDs := strings.Split(zonesString, ",")
	for _, zoneId := range DNSZoneIDs {
		parsedZone, err := azure.ParseResourceID(zoneId)
		if err != nil {
			return fmt.Errorf("while parsing dns zone resource ID %s: %s", zoneId, err)
		}

		if !strings.EqualFold(parsedZone.Provider, "Microsoft.Network") {
			return fmt.Errorf("invalid resource provider %s from zone %s: resource ID must be a public or private DNS Zone resource ID from provider Microsoft.Network", parsedZone.Provider, zoneId)
		}

		switch strings.ToLower(parsedZone.ResourceType) {
		case PrivateZoneType:
			// it's a private zone
			if err := validateSubAndRg(parsedZone, c.PrivateZoneConfig.Subscription, c.PrivateZoneConfig.ResourceGroup); err != nil {
				return err
			}

			c.PrivateZoneConfig.Subscription = parsedZone.SubscriptionID
			c.PrivateZoneConfig.ResourceGroup = parsedZone.ResourceGroup
			c.PrivateZoneConfig.ZoneIds = append(c.PrivateZoneConfig.ZoneIds, zoneId)
		case PublicZoneType:
			// it's a public zone
			if err := validateSubAndRg(parsedZone, c.PublicZoneConfig.Subscription, c.PublicZoneConfig.ResourceGroup); err != nil {
				return err
			}

			c.PublicZoneConfig.Subscription = parsedZone.SubscriptionID
			c.PublicZoneConfig.ResourceGroup = parsedZone.ResourceGroup
			c.PublicZoneConfig.ZoneIds = append(c.PublicZoneConfig.ZoneIds, zoneId)
		default:
			return fmt.Errorf("while parsing dns zone resource ID %s: detected invalid resource type %s", zoneId, parsedZone.ResourceType)
		}
	}

	return nil
}

func validateSubAndRg(parsedZone azure.Resource, subscription, resourceGroup string) error {
	if subscription != "" && parsedZone.SubscriptionID != subscription {
		return fmt.Errorf("while parsing resource IDs for %s: detected multiple subscriptions %s and %s", parsedZone.ResourceType, parsedZone.SubscriptionID, subscription)
	}

	if resourceGroup != "" && parsedZone.ResourceGroup != resourceGroup {
		return fmt.Errorf("while parsing resource IDs for %s: detected multiple resource groups %s and %s", parsedZone.ResourceType, parsedZone.ResourceGroup, resourceGroup)
	}

	return nil
}
