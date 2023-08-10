package clients

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/files"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
)

type acr struct {
	id             string
	name           string
	resourceGroup  string
	subscriptionId string
}

func LoadAcr(id arm.ResourceID) *acr {
	return &acr{
		id:             id.String(),
		name:           id.Name,
		resourceGroup:  id.ResourceGroupName,
		subscriptionId: id.SubscriptionID,
	}
}

func NewAcr(ctx context.Context, subscriptionId, resourceGroup, name, location string) (*acr, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	lgr := logger.FromContext(ctx).With("name", name, "resourceGroup", resourceGroup, "location", location)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create acr")
	defer lgr.Info("finished creating acr")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armcontainerregistry.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	r := &armcontainerregistry.Registry{
		Location: to.Ptr(location),
		SKU: &armcontainerregistry.SKU{
			Name: to.Ptr(armcontainerregistry.SKUNameBasic),
		},
	}
	poller, err := factory.NewRegistriesClient().BeginCreate(ctx, resourceGroup, name, *r, nil)
	if err != nil {
		return nil, fmt.Errorf("starting to create registry: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("waiting for registry creation to complete: %w", err)
	}

	return &acr{
		id:             *result.ID,
		name:           *result.Name,
		resourceGroup:  resourceGroup,
		subscriptionId: subscriptionId,
	}, nil
}

func (a *acr) GetName() string {
	return a.name
}

func (a *acr) GetId() string {
	return a.id
}

func (a *acr) BuildAndPush(ctx context.Context, imageName string) error {
	lgr := logger.FromContext(ctx).With("image", imageName, "name", a.name, "resourceGroup", a.resourceGroup, "subscriptionId", a.subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to build and push image")
	defer lgr.Info("finished building and pushing image")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armcontainerregistry.NewClientFactory(a.subscriptionId, cred, nil)
	if err != nil {
		return fmt.Errorf("creating client factory: %w", err)
	}

	uploadUrlResp, err := factory.NewRegistriesClient().GetBuildSourceUploadURL(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return fmt.Errorf("getting upload url: %w", err)
	}

	uploadUrl := uploadUrlResp.UploadURL
	relativePath := uploadUrlResp.RelativePath

	temp, err := os.MkdirTemp("", "tempdir")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	compressedFile := temp + "/test.tz.gz"
	compressed, err := os.Create(compressedFile)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	targets, err := files.Dir("./")
	if err != nil {
		return fmt.Errorf("getting files to compress: %w", err)
	}
	if err := files.TarGzip(compressed, targets...); err != nil {
		return fmt.Errorf("creating compressed file: %w", err)
	}
	lgr.Info("created compressed file " + compressedFile)
	compressed.Close()

	bc, err := blockblob.NewClient(*uploadUrl, cred, nil)
	if err != nil {
		return fmt.Errorf("creating block blob client: %w", err)
	}

	compressed, err = os.Open(compressedFile)
	if err != nil {
		return fmt.Errorf("opening compressed file: %w", err)
	}
	defer compressed.Close()

	if _, err := bc.UploadFile(ctx, compressed, nil); err != nil {
		return fmt.Errorf("uploading compressed file: %w", err)
	}

	poller, err := factory.NewTaskRunsClient().BeginCreate(ctx, a.resourceGroup, a.name, "taskName", armcontainerregistry.TaskRun{
		Location: nil,
		Properties: &armcontainerregistry.TaskRunProperties{
			RunRequest: &armcontainerregistry.DockerBuildRequest{
				Platform: &armcontainerregistry.PlatformProperties{
					// this should match the go build in the ../Dockerfile
					OS:           to.Ptr(armcontainerregistry.OSLinux),
					Architecture: to.Ptr(armcontainerregistry.ArchitectureAmd64),
				},
				ImageNames:     []*string{to.Ptr(imageName)},
				DockerFilePath: to.Ptr("./"),
				IsPushEnabled:  to.Ptr(true),
				Type:           to.Ptr("DockerBuildRequest"),
				SourceLocation: relativePath,
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("starting to build image: %w", err)
	}

	if _, err = pollWithLog(ctx, poller, "still building image "+imageName); err != nil {
		return fmt.Errorf("building and pushing image: %w", err)
	}

	return nil
}
