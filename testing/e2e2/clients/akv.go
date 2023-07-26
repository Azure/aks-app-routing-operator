package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
)

type akv struct {
	uri string
	id  string
}

// CertOpt specifies what kind of certificate to create
type CertOpt func(cert *azcertificates.CreateCertificateParameters) error

type Cert struct {
	name string
}

func NewAkv(ctx context.Context, subscriptionId, resourceGroup, name, location string) (*akv, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create akv " + name)
	defer lgr.Info("finished creating akv " + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armkeyvault.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	new := &armkeyvault.VaultCreateOrUpdateParameters{
		Location: util.StringPtr(location),
	}
	poller, err := factory.NewVaultsClient().BeginCreateOrUpdate(ctx, resourceGroup, name, *new, nil)
	if err != nil {
		return nil, fmt.Errorf("starting to create vault: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("waiting for vault creation to complete: %w", err)
	}

	return &akv{
		uri: *result.Properties.VaultURI,
		id:  *result.ID,
	}, nil
}

func (a *akv) GetId() string {
	return a.id
}

func (a *akv) CreateCertificate(ctx context.Context, name string, dnsnames []string, certOpts ...CertOpt) (*Cert, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create certificate")
	defer lgr.Info("finished creating certificate")

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := azcertificates.NewClient(a.uri, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	dnsnamesPtr := make([]*string, len(dnsnames))
	for i, dnsname := range dnsnames {
		dnsnamesPtr[i] = to.Ptr(dnsname)
	}
	new := &azcertificates.CreateCertificateParameters{
		CertificateAttributes: nil,
		CertificatePolicy: &azcertificates.CertificatePolicy{
			Attributes: nil,
			IssuerParameters: &azcertificates.IssuerParameters{
				Name: to.Ptr("Self"),
			},
			KeyProperties: &azcertificates.KeyProperties{
				Exportable: to.Ptr(true),
				KeySize:    to.Ptr(int32(2048)),
				KeyType:    to.Ptr(azcertificates.KeyTypeRSA),
				ReuseKey:   to.Ptr(true),
			},
			LifetimeActions: []*azcertificates.LifetimeAction{
				{
					Action: &azcertificates.LifetimeActionType{
						ActionType: to.Ptr(azcertificates.CertificatePolicyActionAutoRenew),
					},
					Trigger: &azcertificates.LifetimeActionTrigger{
						DaysBeforeExpiry: to.Ptr(int32(30)),
					},
				},
			},
			SecretProperties: &azcertificates.SecretProperties{
				ContentType: to.Ptr("application/x-pem-file"),
			},
			X509CertificateProperties: &azcertificates.X509CertificateProperties{
				KeyUsage: []*azcertificates.KeyUsageType{
					to.Ptr(azcertificates.KeyUsageTypeCRLSign),
					to.Ptr(azcertificates.KeyUsageTypeDataEncipherment),
					to.Ptr(azcertificates.KeyUsageTypeDigitalSignature),
					to.Ptr(azcertificates.KeyUsageTypeKeyAgreement),
					to.Ptr(azcertificates.KeyUsageTypeKeyCertSign),
					to.Ptr(azcertificates.KeyUsageTypeKeyEncipherment),
				},
				Subject: to.Ptr("CN=testcert"),
				SubjectAlternativeNames: &azcertificates.SubjectAlternativeNames{
					DNSNames: dnsnamesPtr,
				},
				ValidityInMonths: to.Ptr(int32(12)),
			},
			ID: nil,
		},
		Tags: nil,
	}
	for _, opt := range certOpts {
		if err := opt(new); err != nil {
			return nil, fmt.Errorf("applying certificate option: %w", err)
		}
	}

	_, err = client.CreateCertificate(ctx, name, *new, nil)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	return &Cert{name: name}, nil
}

func (c *Cert) GetName() string {
	return c.name
}
