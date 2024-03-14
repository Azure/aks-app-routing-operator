package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/Azure/go-autorest/autorest/azure"
	"golang.org/x/exp/slog"
)

var (
	cantFindAcrRegex = regexp.MustCompile(`The resource with name '.+' and type 'Microsoft\.ContainerRegistry\/registries' could not be found in subscription`)
	throttledRegex   = regexp.MustCompile(`(SubscriptionRequestsThrottled) Number of requests for subscription '.+' and operation '.+' exceeded the backend storage limit.`)
	hostRegex        = regexp.MustCompile(`dial tcp: lookup .+ on .+: no such host`)
)

type acr struct {
	id             string
	name           string
	resourceGroup  string
	subscriptionId string
}

func LoadAcr(id azure.Resource) *acr {
	return &acr{
		id:             id.String(),
		name:           id.ResourceName,
		resourceGroup:  id.ResourceGroup,
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

	// guard against things that should be impossible
	if result.ID == nil {
		return nil, fmt.Errorf("id is nil")
	}
	if result.Name == nil {
		return nil, fmt.Errorf("name is nil")
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

func (a *acr) BuildAndPush(ctx context.Context, imageName, dockerfilePath, dockerFileName string) error {
	lgr := logger.FromContext(ctx).With("image", imageName, "name", a.name, "resourceGroup", a.resourceGroup, "subscriptionId", a.subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to build and push image")
	defer lgr.Info("finished building and pushing image")

	start := time.Now()

	for {
		// Ideally, we'd use the sdk to build and push the image but I couldn't get it working.
		// I matched everything on the az cli but wasn't able to get it working with the sdk.
		// https://github.com/Azure/azure-cli/blob/5f9a8fa25cc1c980ebe5e034bd419c95a1c578e2/src/azure-cli/azure/cli/command_modules/acr/build.py#L25
		cmd := exec.Command("az", "acr", "build", "--registry", a.name, "--image", imageName, "--subscription", a.subscriptionId, "--resource-group", a.resourceGroup, "--file", dockerFileName, dockerfilePath)
		cmd.Stdout = newLogWriter(lgr, "building and pushing acr image: ", nil)
		var errLog bytes.Buffer
		cmd.Stderr = io.MultiWriter(&errLog, newLogWriter(lgr, "building and pushing acr image: ", to.Ptr(slog.LevelError)))
		err := cmd.Run()
		if err == nil {
			break
		} else {
			// if this regex matches the az cli can't find the acr, things just need more time to propagate.
			// We've tried alternate strategies like polling the sdk to see if the acr exists but that
			// tells us it exists then the acr command fails. This is the only reliable way we've found
			// to wait for the acr to be ready.
			if (!cantFindAcrRegex.Match(errLog.Bytes())) && (!throttledRegex.Match(errLog.Bytes())) && (!hostRegex.Match(errLog.Bytes())) {
				lgr.Error("failed to build and push acr image")
				return fmt.Errorf("running build and push command: %w", err)
			}

			if time.Since(start) > 5*time.Minute {
				return fmt.Errorf("acr not found after 1 minute")
			}

			lgr.Info("waiting for acr to be ready, retrying")
			time.Sleep(5 * time.Second) // longer timeout to account for throttling
		}
	}

	return nil
}
