package clients

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"github.com/Azure/go-autorest/autorest/azure"
)

type akv struct {
	uri            string
	id             string
	tenantId       string
	subscriptionId string
	resourceGroup  string
	name           string
}

// CertOpt specifies what kind of certificate to create
type CertOpt func(cert *azcertificates.CreateCertificateParameters) error

type Cert struct {
	id   string
	name string
}

func LoadAkv(id azure.Resource) *akv {
	return &akv{
		id:             id.String(),
		name:           id.ResourceName,
		resourceGroup:  id.ResourceGroup,
		subscriptionId: id.SubscriptionID,
	}
}

func NewAkv(ctx context.Context, tenantId, subscriptionId, resourceGroup, name, location string) (*akv, error) {
	name = truncate(name, 24)

	lgr := logger.FromContext(ctx).With("name", name, "resourceGroup", resourceGroup, "location", location, "subscriptionId", subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create akv")
	defer lgr.Info("finished creating akv")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armkeyvault.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	clientObjectId, err := getObjectId(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("getting client object id: %w", err)
	}

	v := &armkeyvault.VaultCreateOrUpdateParameters{
		Location: to.Ptr(location),
		Properties: &armkeyvault.VaultProperties{
			AccessPolicies: []*armkeyvault.AccessPolicyEntry{
				{
					ObjectID: to.Ptr(clientObjectId),
					Permissions: &armkeyvault.Permissions{
						Certificates: []*armkeyvault.CertificatePermissions{
							to.Ptr(armkeyvault.CertificatePermissionsCreate),
						},
					},
					TenantID:      to.Ptr(tenantId),
					ApplicationID: nil,
				},
			},
			TenantID: to.Ptr(tenantId),
			SKU: &armkeyvault.SKU{
				Name: to.Ptr(armkeyvault.SKUNameStandard),
			},
		},
	}
	poller, err := factory.NewVaultsClient().BeginCreateOrUpdate(ctx, resourceGroup, name, *v, nil)
	if err != nil {
		return nil, fmt.Errorf("starting to create vault: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("waiting for vault creation to complete: %w", err)
	}

	// guard against things that should be impossible
	if result.Properties == nil {
		return nil, fmt.Errorf("vault properties are nil")
	}
	if result.Properties.VaultURI == nil {
		return nil, fmt.Errorf("vault uri is nil")
	}
	if result.ID == nil {
		return nil, fmt.Errorf("vault id is nil")
	}

	return &akv{
		uri:            *result.Properties.VaultURI,
		id:             *result.ID,
		resourceGroup:  resourceGroup,
		name:           *result.Name,
		subscriptionId: subscriptionId,
		tenantId:       tenantId,
	}, nil
}

func (a *akv) GetId() string {
	return a.id
}

func (a *akv) AddAccessPolicy(ctx context.Context, objectId string, permissions armkeyvault.Permissions) error {
	lgr := logger.FromContext(ctx).With("objectId", objectId, "name", a.name, "resourceGroup", a.resourceGroup, "subscriptionId", a.subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to add access policy")
	defer lgr.Info("finished adding access policy")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armkeyvault.NewVaultsClient(a.subscriptionId, cred, nil)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	addition := armkeyvault.VaultAccessPolicyParameters{
		Properties: &armkeyvault.VaultAccessPolicyProperties{
			AccessPolicies: []*armkeyvault.AccessPolicyEntry{
				{
					TenantID:    to.Ptr(a.tenantId),
					ObjectID:    to.Ptr(objectId),
					Permissions: &permissions,
				},
			},
		},
	}
	if _, err := client.UpdateAccessPolicy(ctx, a.resourceGroup, a.name, armkeyvault.AccessPolicyUpdateKindAdd, addition, nil); err != nil {
		return fmt.Errorf("adding access policy: %w", err)
	}

	return nil
}

func LoadCert(name, id string) *Cert {
	return &Cert{
		id:   id,
		name: name,
	}
}

func (a *akv) CreateCertificate(ctx context.Context, name string, dnsnames []string, certOpts ...CertOpt) (*Cert, error) {
	lgr := logger.FromContext(ctx).With("name", name, "dnsnames", dnsnames, "resourceGroup", a.resourceGroup, "subscriptionId", a.subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create certificate")
	defer lgr.Info("finished creating certificate")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := azcertificates.NewClient(a.uri, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	dnsnamesPtr := to.SliceOfPtrs[string](dnsnames...)
	c := &azcertificates.CreateCertificateParameters{
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
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("applying certificate option: %w", err)
		}
	}

	created, err := client.CreateCertificate(ctx, name, *c, nil)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	// guard against things that should be impossible
	if created.ID == nil {
		return nil, fmt.Errorf("created certificate has nil id")
	}

	id := string(*created.ID)
	id = strings.TrimRight(id, "/pending") // haven't found a better way of getting the cert id other than this so far
	return &Cert{
		id:   id,
		name: name,
	}, nil
}

func (c *Cert) GetName() string {
	return c.name
}

func (c *Cert) GetId() string {
	return c.id
}
