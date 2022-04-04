// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"flag"
)

var Flags = &Config{}

func init() {
	flag.StringVar(&Flags.NS, "namespace", "app-routing-system", "namespace for managed resources")
	flag.StringVar(&Flags.Registry, "registry", "mcr.microsoft.com", "container image registry to use for managed components")
	flag.StringVar(&Flags.MSIClientID, "msi", "", "client ID of the MSI to use when accessing Azure resources")
	flag.StringVar(&Flags.TenantID, "tenant-id", "", "AAD tenant ID to use when accessing Azure resources")
	flag.StringVar(&Flags.Cloud, "cloud", "AzurePublicCloud", "azure cloud name")
	flag.StringVar(&Flags.Location, "location", "", "azure region name")
	flag.StringVar(&Flags.DNSZoneRG, "dns-zone-resource-group", "", "resource group of the Azure DNS Zone (optional)")
	flag.StringVar(&Flags.DNSZoneSub, "dns-zone-subscription", "", "subscription ID of the Azure DNS Zone (optional)")
	flag.StringVar(&Flags.DNSZoneDomain, "dns-zone-domain", "", "domain hostname of the Azure DNS Zone (optional)")
	flag.StringVar(&Flags.DNSRecordID, "dns-record-id", "aks-app-routing-operator", "string that uniquely identifies DNS records managed by this cluster (optional)")
	flag.BoolVar(&Flags.DisableKeyvault, "disable-keyvault", false, "disable the keyvault integration")
}

type Config struct {
	NS, Registry                                      string
	DisableKeyvault                                   bool
	MSIClientID, TenantID                             string
	Cloud, Location                                   string
	DNSZoneRG, DNSZoneSub, DNSZoneDomain, DNSRecordID string
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
	if c.DNSZoneRG != "" || c.DNSZoneSub != "" || c.DNSZoneDomain != "" {
		if c.DNSZoneRG == "" {
			return errors.New("--dns-zone-resource-group is required")
		}
		if c.DNSZoneSub == "" {
			return errors.New("--dns-zone-subscription is required")
		}
		if c.DNSZoneDomain == "" {
			return errors.New("--dns-zone-domain is required")
		}
	}
	return nil
}
