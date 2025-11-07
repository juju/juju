// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v6"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/multiwatcher"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.apiserver.client")

// Client serves client-specific API methods.
type Client struct {
	stateAccessor   Backend
	storageAccessor StorageInterface
	auth            facade.Authorizer
	resources       facade.Resources
	presence        facade.Presence

	multiwatcherFactory multiwatcher.Factory
	leadershipReader    leadership.Reader
	modelCache          *cache.Model
}

// TODO(wallyworld) - remove this method
// state returns a state.State instance for this API.
// Until all code is refactored to use interfaces, we
// need this helper to keep older code happy.
func (c *Client) state() *state.State {
	return c.stateAccessor.(*stateShim).State
}

// ClientV7 serves the (v7) client-specific API methods.
type ClientV7 struct {
	*Client
	registryAPIFunc func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
	toolsFinder     common.ToolsFinder
}

// ClientV6 serves the (v6) client-specific API methods.
type ClientV6 struct {
	*ClientV7
}

func (c *Client) checkCanRead() error {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.auth.HasPermission(permission.ReadAccess, c.stateAccessor.ModelTag())
}

func (c *Client) checkCanWrite() error {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.auth.HasPermission(permission.WriteAccess, c.stateAccessor.ModelTag())
}

func (c *Client) checkIsAdmin() error {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.auth.HasPermission(permission.AdminAccess, c.stateAccessor.ModelTag())
}

// NewFacadeV7 creates a ClientV7 facade to handle API requests.
// Changes:
// - FindTools deals with CAAS models now;
func NewFacadeV7(ctx facade.Context) (*ClientV7, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()
	presence := ctx.Presence()
	factory := ctx.MultiwatcherFactory()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	newEnviron := common.EnvironFuncForModel(model, configGetter)

	modelUUID := model.UUID()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	urlGetter := common.NewToolsURLGetter(modelUUID, systemState)
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
	leadershipReader, err := ctx.LeadershipReader(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCache, err := ctx.CachedModel(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClientV7(
		&stateShim{st, model, nil},
		storageAccessor,
		resources,
		authorizer,
		presence,
		toolsFinder,
		leadershipReader,
		modelCache,
		factory,
		registry.New,
	)
}

// NewClientV7 creates a new instance of the ClientV7 Facade.
func NewClientV7(
	backend Backend,
	storageAccessor StorageInterface,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	toolsFinder common.ToolsFinder,
	leadershipReader leadership.Reader,
	modelCache *cache.Model,
	factory multiwatcher.Factory,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
) (*ClientV7, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &ClientV7{
		Client: &Client{
			stateAccessor:       backend,
			storageAccessor:     storageAccessor,
			auth:                authorizer,
			resources:           resources,
			presence:            presence,
			leadershipReader:    leadershipReader,
			modelCache:          modelCache,
			multiwatcherFactory: factory,
		},
		registryAPIFunc: registryAPIFunc,
		toolsFinder:     toolsFinder,
	}, nil
}

// WatchAll initiates a watcher for entities in the connected model.
func (c *Client) WatchAll() (params.AllWatcherId, error) {
	if err := c.checkCanRead(); err != nil {
		return params.AllWatcherId{}, err
	}
	isAdmin, err := common.HasModelAdmin(c.auth, c.stateAccessor.ControllerTag(), names.NewModelTag(c.state().ModelUUID()))
	if err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}
	modelUUID := c.stateAccessor.ModelUUID()
	w := c.multiwatcherFactory.WatchModel(modelUUID)
	if !isAdmin {
		w = &stripApplicationOffers{w}
	}
	return params.AllWatcherId{
		AllWatcherId: c.resources.Register(w),
	}, nil
}

type stripApplicationOffers struct {
	multiwatcher.Watcher
}

func (s *stripApplicationOffers) Next() ([]multiwatcher.Delta, error) {
	var result []multiwatcher.Delta
	// We don't want to return a list on nothing. Next normally blocks until there
	// is something to return.
	for len(result) == 0 {
		deltas, err := s.Watcher.Next()
		if err != nil {
			return nil, err
		}
		result = make([]multiwatcher.Delta, 0, len(deltas))
		for _, d := range deltas {
			switch d.Entity.EntityID().Kind {
			case multiwatcher.ApplicationOfferKind:
				// skip it
			default:
				result = append(result, d)
			}
		}
	}
	return result, nil
}

// FindTools returns a List containing all tools matching the given parameters.
// TODO(juju 3.1) - remove, used by 2.9 client only
func (c *ClientV7) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.FindToolsResult{}, err
	}
	model, err := c.stateAccessor.Model()
	if err != nil {
		return params.FindToolsResult{}, errors.Trace(err)
	}

	list, err := c.toolsFinder.FindAgents(common.FindAgentsParams{
		Number:       args.Number,
		MajorVersion: args.MajorVersion,
		Arch:         args.Arch,
		OSType:       args.OSType,
		AgentStream:  args.AgentStream,
	})
	result := params.FindToolsResult{
		List:  list,
		Error: apiservererrors.ServerError(err),
	}
	if model.Type() != state.ModelTypeCAAS {
		// We return now for non CAAS model.
		return result, errors.Annotate(err, "finding tool version from simple streams")
	}
	// Continue to check agent image tags via registry API for CAAS model.
	if err != nil && !errors.IsNotFound(err) || result.Error != nil && !params.IsCodeNotFound(result.Error) {
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

func (c *ClientV7) toolVersionsForCAAS(args params.FindToolsParams, streamsVersions set.Strings, current version.Number) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	controllerCfg, err := c.stateAccessor.ControllerConfig()
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
		arches, err := reg.GetArchitectures(imageName, number.String())
		if errors.IsNotFound(err) {
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
