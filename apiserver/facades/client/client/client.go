// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = internallogger.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor           Backend
	pool                    Pool
	storageAccessor         StorageInterface
	blockDeviceService      BlockDeviceService
	controllerConfigService ControllerConfigService
	auth                    facade.Authorizer
	resources               facade.Resources
	presence                facade.Presence

	toolsFinder      common.ToolsFinder
	leadershipReader leadership.Reader
	networkService   NetworkService
}

// TODO(wallyworld) - remove this method
// state returns a state.State instance for this API.
// Until all code is refactored to use interfaces, we
// need this helper to keep older code happy.
func (api *API) state() *state.State {
	return api.stateAccessor.(*stateShim).State
}

// Client serves client-specific API methods.
type Client struct {
	api              *API
	newEnviron       common.NewEnvironFunc
	check            *common.BlockChecker
	registryAPIFunc  func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
	modelInfoService ModelInfoService
}

// ClientV6 serves the (v6) client-specific API methods.
type ClientV6 struct {
	*Client
}

func (c *Client) checkCanRead(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.ReadAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkCanWrite(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.WriteAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkIsAdmin(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.AdminAccess, c.api.stateAccessor.ModelTag())
}

// NewFacade creates a Client facade to handle API requests.
// Changes:
// - FindTools deals with CAAS models now;
func NewFacade(ctx facade.ModelContext) (*Client, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()
	presence := ctx.Presence()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:             model,
		CloudService:      serviceFactory.Cloud(),
		CredentialService: serviceFactory.Credential(),
	}
	newEnviron := common.EnvironFuncForModel(model, serviceFactory.Cloud(), serviceFactory.Credential(), configGetter)

	modelUUID := model.UUID()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService := serviceFactory.ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigService, st, urlGetter, newEnviron, ctx.ControllerObjectStore())
	blockChecker := common.NewBlockChecker(st)
	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClient(
		&stateShim{
			State:                    st,
			model:                    model,
			session:                  nil,
			configSchemaSourceGetter: environs.ProviderConfigSchemaSource(serviceFactory.Cloud()),
		},
		ctx.ServiceFactory().ModelInfo(),
		&poolShim{pool: ctx.StatePool()},
		storageAccessor,
		serviceFactory.BlockDevice(),
		controllerConfigService,
		resources,
		authorizer,
		presence,
		toolsFinder,
		newEnviron,
		blockChecker,
		leadershipReader,
		ctx.ServiceFactory().Network(),
		registry.New,
	)
}

// NewClient creates a new instance of the Client Facade.
// TODO(aflynn): Create an args struct for this.
func NewClient(
	backend Backend,
	modelInfoService ModelInfoService,
	pool Pool,
	storageAccessor StorageInterface,
	blockDeviceService BlockDeviceService,
	controllerConfigService ControllerConfigService,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	toolsFinder common.ToolsFinder,
	newEnviron common.NewEnvironFunc,
	blockChecker *common.BlockChecker,
	leadershipReader leadership.Reader,
	networkService NetworkService,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &Client{
		api: &API{
			stateAccessor:           backend,
			pool:                    pool,
			storageAccessor:         storageAccessor,
			blockDeviceService:      blockDeviceService,
			controllerConfigService: controllerConfigService,
			auth:                    authorizer,
			resources:               resources,
			presence:                presence,
			toolsFinder:             toolsFinder,
			leadershipReader:        leadershipReader,
			networkService:          networkService,
		},
		modelInfoService: modelInfoService,
		newEnviron:       newEnviron,
		check:            blockChecker,
		registryAPIFunc:  registryAPIFunc,
	}
	return client, nil
}

// WatchAll initiates a watcher for entities in the connected model.
func (c *Client) WatchAll(ctx context.Context) (params.AllWatcherId, error) {
	return params.AllWatcherId{}, errors.NotImplementedf("WatchAll")
}

// FindTools returns a List containing all tools matching the given parameters.
// TODO(juju 3.1) - remove, used by 2.9 client only
func (c *Client) FindTools(ctx context.Context, args params.FindToolsParams) (params.FindToolsResult, error) {
	if err := c.checkCanWrite(ctx); err != nil {
		return params.FindToolsResult{}, err
	}
	model, err := c.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.FindToolsResult{}, errors.Trace(err)
	}

	list, err := c.api.toolsFinder.FindAgents(
		ctx,
		common.FindAgentsParams{
			Number:       args.Number,
			MajorVersion: args.MajorVersion,
			Arch:         args.Arch,
			OSType:       args.OSType,
			AgentStream:  args.AgentStream,
		},
	)
	result := params.FindToolsResult{
		List:  list,
		Error: apiservererrors.ServerError(err),
	}

	if model.Type != coremodel.CAAS {
		// We return now for non CAAS model.
		return result, errors.Annotate(err, "finding tool version from simple streams")
	}
	// Continue to check agent image tags via registry API for CAAS model.
	if err != nil && !errors.Is(err, errors.NotFound) || result.Error != nil && !params.IsCodeNotFound(result.Error) {
		return result, errors.Annotate(err, "finding tool versions from simplestream")
	}
	streamsVersions := set.NewStrings()
	for _, a := range result.List {
		streamsVersions.Add(a.Version.Number.String())
	}
	logger.Tracef("versions from simplestream %v", streamsVersions.SortedValues())
	return c.toolVersionsForCAAS(ctx, args, streamsVersions, model.AgentVersion)
}

func (c *Client) toolVersionsForCAAS(ctx context.Context, args params.FindToolsParams, streamsVersions set.Strings, current version.Number) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	controllerCfg, err := c.api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	imageRepoDetails, err := docker.NewImageRepoDetails(controllerCfg.CAASImageRepo())
	if err != nil {
		return result, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	if imageRepoDetails.Empty() {
		imageRepoDetails, err = docker.NewImageRepoDetails(podcfg.JujudOCINamespace)
		if err != nil {
			return result, errors.Trace(err)
		}
	}
	reg, err := c.registryAPIFunc(imageRepoDetails)
	if err != nil {
		return result, errors.Annotatef(err, "constructing registry API for %s", imageRepoDetails)
	}
	defer func() { _ = reg.Close() }()
	imageName := podcfg.JujudOCIName
	tags, err := reg.Tags(imageName)
	if err != nil {
		return result, errors.Trace(err)
	}

	wantArch := args.Arch
	if wantArch == "" {
		wantArch = arch.DefaultArchitecture
	}
	for _, tag := range tags {
		number := tag.AgentVersion()
		if number.Compare(current) <= 0 {
			continue
		}
		if current.Build == 0 && number.Build > 0 {
			continue
		}
		if args.MajorVersion != -1 && number.Major != args.MajorVersion {
			continue
		}
		if !controllerCfg.Features().Contains(featureflag.DeveloperMode) && streamsVersions.Size() > 0 {
			numberCopy := number
			numberCopy.Build = 0
			if !streamsVersions.Contains(numberCopy.String()) {
				continue
			}
		} else {
			// Fallback for when we can't query the streams versions.
			// Ignore tagged (non-release) versions if agent stream is released.
			if (args.AgentStream == "" || args.AgentStream == envtools.ReleasedStream) && number.Tag != "" {
				continue
			}
		}
		arches, err := reg.GetArchitectures(imageName, number.String())
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return result, errors.Annotatef(err, "cannot get architecture for %s:%s", imageName, number.String())
		}
		if !set.NewStrings(arches...).Contains(wantArch) {
			continue
		}
		tools := tools.Tools{
			Version: version.Binary{
				Number:  number,
				Release: coreos.HostOSTypeName(),
				Arch:    wantArch,
			},
		}
		result.List = append(result.List, &tools)
	}
	return result, nil
}

// NOTE: this is necessary for the other packages that do upgrade tests.
// Really they should be using a mocked out api server, but that is outside
// the scope of this fix.
var skipReplicaCheck = false

// SkipReplicaCheck is required for tests only as the test mongo isn't a replica.
func SkipReplicaCheck(patcher Patcher) {
	patcher.PatchValue(&skipReplicaCheck, true)
}

// Patcher is provided by the test suites to temporarily change values.
type Patcher interface {
	PatchValue(dest, value interface{})
}
