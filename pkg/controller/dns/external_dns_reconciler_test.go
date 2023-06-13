package dns

import (
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/require"
)

var (
	privateZoneOne = "/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-one.com"
	privateZoneTwo = "/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-two.com"
	privateZones   = []string{privateZoneOne, privateZoneTwo}

	publicZoneOne = "/subscriptions/test-public-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-one.com"
	publicZoneTwo = "/subscriptions/test-public-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-two.com"
	publicZones   = []string{publicZoneOne, publicZoneTwo}

	zones = strings.Join(append(privateZones, publicZones...), ",")

	publicConfig = &config.Config{
		NS:                       "test-ns",
		DisableKeyvault:          false,
		PrivateZoneIds:           nil,
		PublicZoneIds:            publicZones,
		PrivateZoneSubscription:  "",
		PublicZoneSubscription:   "test-public-subscription",
		PrivateZoneResourceGroup: "",
		PublicZoneResourceGroup:  "test-public-rg",
	}
	privateConfig = &config.Config{
		NS:                       "test-ns",
		DisableKeyvault:          false,
		PrivateZoneIds:           privateZones,
		PublicZoneIds:            nil,
		PrivateZoneSubscription:  "test-private-subscription",
		PublicZoneSubscription:   "",
		PrivateZoneResourceGroup: "test-private-rg",
		PublicZoneResourceGroup:  "",
	}
	fullConfig = &config.Config{
		NS:                       "test-ns",
		DisableKeyvault:          false,
		PrivateZoneIds:           privateZones,
		PublicZoneIds:            publicZones,
		PrivateZoneSubscription:  "test-private-subscription",
		PublicZoneSubscription:   "test-public-subscription",
		PrivateZoneResourceGroup: "test-private-rg",
		PublicZoneResourceGroup:  "test-public-rg",
	}
)

func TestGenerateZoneConfigs_PublicOnly(t *testing.T) {
	zoneConfigs := generateZoneConfigs(publicConfig)

	require.Equal(t, 1, len(zoneConfigs))
	require.Equal(t, publicConfig.PublicZoneIds, zoneConfigs[0].DnsZoneResourceIDs)
	require.Equal(t, manifests.Provider, zoneConfigs[0].Provider)
	require.Equal(t, publicConfig.PublicZoneSubscription, zoneConfigs[0].Subscription)
}

func TestGenerateZoneConfigs_PrivateOnly(t *testing.T) {
	zoneConfigs := generateZoneConfigs(privateConfig)

	require.Equal(t, 1, len(zoneConfigs))
	require.Equal(t, privateConfig.PrivateZoneIds, zoneConfigs[0].DnsZoneResourceIDs)
	require.Equal(t, manifests.PrivateProvider, zoneConfigs[0].Provider)
	require.Equal(t, privateConfig.PrivateZoneSubscription, zoneConfigs[0].Subscription)
}

func TestGenerateZoneConfigs_All(t *testing.T) {
	zoneConfigs := generateZoneConfigs(fullConfig)

	require.Equal(t, len(zoneConfigs), 2)

	prConfig := zoneConfigs[0]
	pbConfig := zoneConfigs[1]

	require.Equal(t, fullConfig.PrivateZoneIds, prConfig.DnsZoneResourceIDs)
	require.Equal(t, fullConfig.PublicZoneIds, pbConfig.DnsZoneResourceIDs)

	require.Equal(t, manifests.PrivateProvider, prConfig.Provider)
	require.Equal(t, manifests.Provider, pbConfig.Provider)

	require.Equal(t, fullConfig.PrivateZoneSubscription, prConfig.Subscription)
	require.Equal(t, fullConfig.PublicZoneSubscription, pbConfig.Subscription)

}
