// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/feature"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor   Backend
	pool            Pool
	storageAccessor StorageInterface
	auth            facade.Authorizer
	resources       facade.Resources
	presence        facade.Presence

	toolsFinder      common.ToolsFinder
	leadershipReader leadership.Reader
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
	api             *API
	newEnviron      common.NewEnvironFunc
	check           *common.BlockChecker
	registryAPIFunc func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
}

// ClientV6 serves the (v6) client-specific API methods.
type ClientV6 struct {
	*Client
}

func (c *Client) checkCanRead() error {
	err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(permission.ReadAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkCanWrite() error {
	err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(permission.WriteAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkIsAdmin() error {
	err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(permission.AdminAccess, c.api.stateAccessor.ModelTag())
}

// NewFacade creates a Client facade to handle API requests.
// Changes:
// - FindTools deals with CAAS models now;
func NewFacade(ctx facade.Context) (*Client, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()
	presence := ctx.Presence()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configGetter := stateenvirons.EnvironConfigGetter{
		Model: model, CloudService: ctx.ServiceFactory().Cloud(), CredentialService: ctx.ServiceFactory().Credential()}
	newEnviron := common.EnvironFuncForModel(model, ctx.ServiceFactory().Cloud(), ctx.ServiceFactory().Credential(), configGetter)

	modelUUID := model.UUID()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigGetter := ctx.ServiceFactory().ControllerConfig()

	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(controllerConfigGetter, configGetter, st, urlGetter, newEnviron, ctx.ControllerObjectStore())
	blockChecker := common.NewBlockChecker(st)
	leadershipReader, err := ctx.LeadershipReader(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClient(
		&stateShim{st, model, nil},
		&poolShim{ctx.StatePool()},
		storageAccessor,
		resources,
		authorizer,
		presence,
		toolsFinder,
		newEnviron,
		blockChecker,
		leadershipReader,
		registry.New,
	)
}

// NewClient creates a new instance of the Client Facade.
func NewClient(
	backend Backend,
	pool Pool,
	storageAccessor StorageInterface,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	toolsFinder common.ToolsFinder,
	newEnviron common.NewEnvironFunc,
	blockChecker *common.BlockChecker,
	leadershipReader leadership.Reader,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &Client{
		api: &API{
			stateAccessor:    backend,
			pool:             pool,
			storageAccessor:  storageAccessor,
			auth:             authorizer,
			resources:        resources,
			presence:         presence,
			toolsFinder:      toolsFinder,
			leadershipReader: leadershipReader,
		},
		newEnviron:      newEnviron,
		check:           blockChecker,
		registryAPIFunc: registryAPIFunc,
	}
	return client, nil
}

// FindTools returns a List containing all tools matching the given parameters.
// TODO(juju 3.1) - remove, used by 2.9 client only
func (c *Client) FindTools(ctx stdcontext.Context, args params.FindToolsParams) (params.FindToolsResult, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.FindToolsResult{}, err
	}
	model, err := c.api.stateAccessor.Model()
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
	if model.Type() != state.ModelTypeCAAS {
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
	mCfg, err := model.Config()
	if err != nil {
		return result, errors.Annotate(err, "getting model config")
	}
	currentVersion, ok := mCfg.AgentVersion()
	if !ok {
		return result, errors.NotValidf("agent version is not set for model %q", model.Name())
	}
	return c.toolVersionsForCAAS(args, streamsVersions, currentVersion)
}

func (c *Client) toolVersionsForCAAS(args params.FindToolsParams, streamsVersions set.Strings, current version.Number) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	controllerCfg, err := c.api.stateAccessor.ControllerConfig()
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
		if !controllerCfg.Features().Contains(feature.DeveloperMode) && streamsVersions.Size() > 0 {
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
		arch, err := reg.GetArchitecture(imageName, number.String())
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return result, errors.Annotatef(err, "cannot get architecture for %s:%s", imageName, number.String())
		}
		if args.Arch != "" && arch != args.Arch {
			continue
		}
		tools := tools.Tools{
			Version: version.Binary{
				Number:  number,
				Release: coreos.HostOSTypeName(),
				Arch:    arch,
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
