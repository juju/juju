// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/winrmprovisioner"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor Backend
	pool          Pool
	auth          facade.Authorizer
	resources     facade.Resources
	presence      facade.Presence

	multiwatcherFactory multiwatcher.Factory

	toolsFinder      common.ToolsFinder
	leadershipReader leadership.Reader
	modelCache       *cache.Model
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
	*modelconfig.ModelConfigAPIV2

	api             *API
	newEnviron      common.NewEnvironFunc
	check           *common.BlockChecker
	callContext     context.ProviderCallContext
	registryAPIFunc func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
}

func (c *Client) checkCanRead() error {
	isAdmin, err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}

	canRead, err := c.api.auth.HasPermission(permission.ReadAccess, c.api.stateAccessor.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead && !isAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (c *Client) checkCanWrite() error {
	isAdmin, err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}

	canWrite, err := c.api.auth.HasPermission(permission.WriteAccess, c.api.stateAccessor.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite && !isAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (c *Client) checkIsAdmin() error {
	isAdmin, err := c.api.auth.HasPermission(permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}

	isModelAdmin, err := c.api.auth.HasPermission(permission.AdminAccess, c.api.stateAccessor.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isModelAdmin && !isAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

// NewFacade creates a version 4 Client facade to handle API requests.
// Changes:
// - FindTools deals with CAAS models now;
func NewFacade(ctx facade.Context) (*Client, error) {
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

	urlGetter := common.NewToolsURLGetter(modelUUID, ctx.StatePool().SystemState())
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter, newEnviron)
	blockChecker := common.NewBlockChecker(st)
	backend := modelconfig.NewStateBackend(model)
	// The modelConfigAPI exposed here is V1.
	modelConfigAPI, err := modelconfig.NewModelConfigAPI(backend, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	leadershipReader, err := ctx.LeadershipReader(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCache, err := ctx.CachedModel(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClient(
		&stateShim{st, model, nil},
		&poolShim{ctx.StatePool()},
		modelConfigAPI,
		resources,
		authorizer,
		presence,
		toolsFinder,
		newEnviron,
		blockChecker,
		context.CallContext(st),
		leadershipReader,
		modelCache,
		factory,
		registry.New,
	)
}

// NewClient creates a new instance of the Client Facade.
func NewClient(
	backend Backend,
	pool Pool,
	modelConfigAPI *modelconfig.ModelConfigAPIV2,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	toolsFinder common.ToolsFinder,
	newEnviron common.NewEnvironFunc,
	blockChecker *common.BlockChecker,
	callCtx context.ProviderCallContext,
	leadershipReader leadership.Reader,
	modelCache *cache.Model,
	factory multiwatcher.Factory,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &Client{
		ModelConfigAPIV2: modelConfigAPI,
		api: &API{
			stateAccessor:       backend,
			pool:                pool,
			auth:                authorizer,
			resources:           resources,
			presence:            presence,
			toolsFinder:         toolsFinder,
			leadershipReader:    leadershipReader,
			modelCache:          modelCache,
			multiwatcherFactory: factory,
		},
		newEnviron:      newEnviron,
		check:           blockChecker,
		callContext:     callCtx,
		registryAPIFunc: registryAPIFunc,
	}
	return client, nil
}

// WatchAll initiates a watcher for entities in the connected model.
func (c *Client) WatchAll() (params.AllWatcherId, error) {
	if err := c.checkCanRead(); err != nil {
		return params.AllWatcherId{}, err
	}
	isAdmin, err := common.HasModelAdmin(c.api.auth, c.api.stateAccessor.ControllerTag(), names.NewModelTag(c.api.state().ModelUUID()))
	if err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}
	modelUUID := c.api.stateAccessor.ModelUUID()
	w := c.api.multiwatcherFactory.WatchModel(modelUUID)
	if !isAdmin {
		w = &stripApplicationOffers{w}
	}
	return params.AllWatcherId{
		AllWatcherId: c.api.resources.Register(w),
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

// Resolved implements the server side of Client.Resolved.
func (c *Client) Resolved(p params.Resolved) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	unit, err := c.api.stateAccessor.Unit(p.UnitName)
	if err != nil {
		return err
	}
	return unit.Resolve(p.Retry)
}

// PublicAddress implements the server side of Client.PublicAddress.
func (c *Client) PublicAddress(p params.PublicAddress) (results params.PublicAddressResults, err error) {
	if err := c.checkCanRead(); err != nil {
		return params.PublicAddressResults{}, err
	}

	switch {
	case names.IsValidMachine(p.Target):
		machine, err := c.api.stateAccessor.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr, err := machine.PublicAddress()
		if err != nil {
			return results, errors.Annotatef(err, "error fetching address for machine %q", machine)
		}
		return params.PublicAddressResults{PublicAddress: addr.Value}, nil

	case names.IsValidUnit(p.Target):
		unit, err := c.api.stateAccessor.Unit(p.Target)
		if err != nil {
			return results, err
		}
		addr, err := unit.PublicAddress()
		if err != nil {
			return results, errors.Annotatef(err, "error fetching address for unit %q", unit)
		}
		return params.PublicAddressResults{PublicAddress: addr.Value}, nil
	}
	return results, errors.Errorf("unknown unit or machine %q", p.Target)
}

// PrivateAddress implements the server side of Client.PrivateAddress.
func (c *Client) PrivateAddress(p params.PrivateAddress) (results params.PrivateAddressResults, err error) {
	if err := c.checkCanRead(); err != nil {
		return params.PrivateAddressResults{}, err
	}

	switch {
	case names.IsValidMachine(p.Target):
		machine, err := c.api.stateAccessor.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr, err := machine.PrivateAddress()
		if err != nil {
			return results, errors.Annotatef(err, "error fetching address for machine %q", machine)
		}
		return params.PrivateAddressResults{PrivateAddress: addr.Value}, nil

	case names.IsValidUnit(p.Target):
		unit, err := c.api.stateAccessor.Unit(p.Target)
		if err != nil {
			return results, err
		}
		addr, err := unit.PrivateAddress()
		if err != nil {
			return results, errors.Annotatef(err, "error fetching address for unit %q", unit)
		}
		return params.PrivateAddressResults{PrivateAddress: addr.Value}, nil
	}
	return results, fmt.Errorf("unknown unit or machine %q", p.Target)

}

// GetModelConstraints returns the constraints for the model.
func (c *Client) GetModelConstraints() (params.GetConstraintsResults, error) {
	if err := c.checkCanRead(); err != nil {
		return params.GetConstraintsResults{}, err
	}

	cons, err := c.api.stateAccessor.ModelConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *Client) SetModelConstraints(args params.SetConstraints) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.SetModelConstraints(args.Constraints)
}

// AddMachines adds new machines with the supplied parameters.
func (c *Client) AddMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	return c.AddMachinesV2(args)
}

// AddMachinesV2 adds new machines with the supplied parameters.
func (c *Client) AddMachinesV2(args params.AddMachines) (params.AddMachinesResults, error) {
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}
	if err := c.checkCanWrite(); err != nil {
		return params.AddMachinesResults{}, err
	}
	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, p := range args.MachineParams {
		m, err := c.addOneMachine(p)
		results.Machines[i].Error = apiservererrors.ServerError(err)
		if err == nil {
			results.Machines[i].Machine = m.Id()
		}
	}
	return results, nil
}

// InjectMachines injects a machine into state with provisioned status.
func (c *Client) InjectMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	return c.AddMachines(args)
}

func (c *Client) addOneMachine(p params.AddMachineParams) (*state.Machine, error) {
	if p.ParentId != "" && p.ContainerType == "" {
		return nil, fmt.Errorf("parent machine specified without container type")
	}
	if p.ContainerType != "" && p.Placement != nil {
		return nil, fmt.Errorf("container type and placement are mutually exclusive")
	}
	if p.Placement != nil {
		// Extract container type and parent from container placement directives.
		containerType, err := instance.ParseContainerType(p.Placement.Scope)
		if err == nil {
			p.ContainerType = containerType
			p.ParentId = p.Placement.Directive
			p.Placement = nil
		}
	}

	if p.ContainerType != "" || p.Placement != nil {
		// Guard against dubious client by making sure that
		// the following attributes can only be set when we're
		// not using placement.
		p.InstanceId = ""
		p.Nonce = ""
		p.HardwareCharacteristics = instance.HardwareCharacteristics{}
		p.Addrs = nil
	}

	if p.Series == "" {
		conf, err := c.api.stateAccessor.ModelConfig()
		if err != nil {
			return nil, err
		}
		p.Series = config.PreferredSeries(conf)
	}

	var placementDirective string
	if p.Placement != nil {
		model, err := c.api.stateAccessor.Model()
		if err != nil {
			return nil, err
		}
		// For 1.21 we should support both UUID and name, and with 1.22
		// just support UUID
		if p.Placement.Scope != model.Name() && p.Placement.Scope != model.UUID() {
			return nil, fmt.Errorf("invalid model name %q", p.Placement.Scope)
		}
		placementDirective = p.Placement.Directive
	}

	jobs, err := common.StateJobs(p.Jobs)
	if err != nil {
		return nil, err
	}

	addrs, err := params.ToProviderAddresses(p.Addrs...).ToSpaceAddresses(c.api.stateAccessor)
	if err != nil {
		return nil, err
	}

	template := state.MachineTemplate{
		Series:                  p.Series,
		Constraints:             p.Constraints,
		InstanceId:              p.InstanceId,
		Jobs:                    jobs,
		Nonce:                   p.Nonce,
		HardwareCharacteristics: p.HardwareCharacteristics,
		Addresses:               addrs,
		Placement:               placementDirective,
	}
	if p.ContainerType == "" {
		return c.api.stateAccessor.AddOneMachine(template)
	}
	if p.ParentId != "" {
		return c.api.stateAccessor.AddMachineInsideMachine(template, p.ParentId, p.ContainerType)
	}
	return c.api.stateAccessor.AddMachineInsideNewMachine(template, template, p.ContainerType)
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (params.ProvisioningScriptResult, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.ProvisioningScriptResult{}, err
	}

	var result params.ProvisioningScriptResult
	icfg, err := InstanceConfig(c.api.pool.SystemState(), c.api.state(), args.MachineId, args.Nonce, args.DataDir)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting instance config",
		))
	}
	// Until DisablePackageCommands is retired, for backwards
	// compatibility, we must respect the client's request and
	// override any model settings the user may have specified.
	// If the client does specify this setting, it will only ever be
	// true. False indicates the client doesn't care and we should use
	// what's specified in the environment config.
	if args.DisablePackageCommands {
		icfg.EnableOSRefreshUpdate = false
		icfg.EnableOSUpgrade = false
	} else if cfg, err := c.api.stateAccessor.ModelConfig(); err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting model config",
		))
	} else {
		icfg.EnableOSUpgrade = cfg.EnableOSUpgrade()
		icfg.EnableOSRefreshUpdate = cfg.EnableOSRefreshUpdate()
	}

	osSeries, err := series.GetOSFromSeries(icfg.Series)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotatef(err,
			"cannot decide which provisioning script to generate based on this series %q", icfg.Series))
	}

	getProvisioningScript := sshprovisioner.ProvisioningScript
	if osSeries == coreos.Windows {
		getProvisioningScript = winrmprovisioner.ProvisioningScript
	}

	result.Script, err = getProvisioningScript(icfg)
	if err != nil {
		return result, apiservererrors.ServerError(errors.Annotate(
			err, "getting provisioning script",
		))
	}

	return result, nil
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.RemoveAllowed(); !args.Force && err != nil {
		return errors.Trace(err)
	}

	return common.DestroyMachines(c.api.stateAccessor, args.Force, time.Duration(0), args.MachineNames...)
}

// ModelInfo returns information about the current model.
func (c *Client) ModelInfo() (params.ModelInfo, error) {
	if err := c.checkCanRead(); err != nil {
		return params.ModelInfo{}, err
	}
	state := c.api.stateAccessor
	conf, err := state.ModelConfig()
	if err != nil {
		return params.ModelInfo{}, err
	}
	model, err := state.Model()
	if err != nil {
		return params.ModelInfo{}, err
	}

	info := params.ModelInfo{
		DefaultSeries:  config.PreferredSeries(conf),
		CloudTag:       names.NewCloudTag(model.CloudName()).String(),
		CloudRegion:    model.CloudRegion(),
		ProviderType:   conf.Type(),
		Name:           conf.Name(),
		Type:           string(model.Type()),
		UUID:           model.UUID(),
		OwnerTag:       model.Owner().String(),
		Life:           life.Value(model.Life().String()),
		ControllerUUID: state.ControllerTag().String(),
		IsController:   state.IsController(),
	}
	if agentVersion, exists := conf.AgentVersion(); exists {
		info.AgentVersion = &agentVersion
	}
	if tag, ok := model.CloudCredentialTag(); ok {
		info.CloudCredentialTag = tag.String()
	}
	info.SLA = &params.ModelSLAInfo{
		Level: model.SLALevel(),
		Owner: model.SLAOwner(),
	}
	return info, nil
}

func modelInfo(st *state.State, user permission.UserAccess) (params.ModelUserInfo, error) {
	model, err := st.Model()
	if err != nil {
		return params.ModelUserInfo{}, errors.Trace(err)
	}
	return common.ModelUserInfo(user, model)
}

// ModelUserInfo returns information on all users in the model.
func (c *Client) ModelUserInfo() (params.ModelUserInfoResults, error) {
	var results params.ModelUserInfoResults
	if err := c.checkCanRead(); err != nil {
		return results, err
	}

	model, err := c.api.stateAccessor.Model()
	if err != nil {
		return results, errors.Trace(err)
	}
	users, err := model.Users()
	if err != nil {
		return results, errors.Trace(err)
	}

	for _, user := range users {
		var result params.ModelUserInfoResult
		userInfo, err := modelInfo(c.api.state(), user)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = &userInfo
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// AgentVersion returns the current version that the API server is running.
func (c *Client) AgentVersion() (params.AgentVersionResult, error) {
	if err := c.checkCanRead(); err != nil {
		return params.AgentVersionResult{}, err
	}

	return params.AgentVersionResult{Version: jujuversion.Current}, nil
}

// SetModelAgentVersion sets the model agent version.
func (c *Client) SetModelAgentVersion(args params.SetModelAgentVersion) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Before changing the agent version to trigger an upgrade or downgrade,
	// we'll do a very basic check to ensure the environment is accessible.
	envOrBroker, err := c.newEnviron()
	if err != nil {
		return errors.Trace(err)
	}
	// Check IAAS clouds.
	if env, ok := envOrBroker.(environs.InstanceBroker); ok {
		if err := environs.CheckProviderAPI(env, c.callContext); err != nil {
			return err
		}
	}
	// Check credentials against the container broker
	if env, ok := envOrBroker.(caas.CredentialChecker); ok {
		if err := env.CheckCloudCredentials(); err != nil {
			return errors.Annotate(err, "cannot make API call to provider")
		}
	}
	// If this is the controller model, also check to make sure that there are
	// no running migrations.  All models should have migration mode of None.
	// For major version upgrades, also check that all models are at a version high
	// enough to allow the upgrade.
	if c.api.stateAccessor.IsController() {
		// Check to ensure that the replicaset is happy.
		if err := c.CheckMongoStatusForUpgrade(c.api.stateAccessor.MongoSession()); err != nil {
			return errors.Trace(err)
		}

		modelUUIDs, err := c.api.stateAccessor.AllModelUUIDs()
		if err != nil {
			return errors.Trace(err)
		}

		var oldModels []string
		var requiredVersion version.Number
		for _, modelUUID := range modelUUIDs {
			model, release, err := c.api.pool.GetModel(modelUUID)
			if err != nil {
				return errors.Trace(err)
			}
			vers, err := model.AgentVersion()
			if err != nil {
				return errors.Trace(err)
			}
			allowed, minVer, err := upgrades.UpgradeAllowed(vers, args.Version)
			if err != nil {
				return errors.Trace(err)
			}
			if !allowed {
				requiredVersion = minVer
				oldModels = append(oldModels, fmt.Sprintf("%s/%s", model.Owner().Name(), model.Name()))
			}
			if mode := model.MigrationMode(); mode != state.MigrationModeNone {
				release()
				return errors.Errorf("model \"%s/%s\" is %s, upgrade blocked", model.Owner().Name(), model.Name(), mode)
			}
			release()
		}
		if len(oldModels) > 0 {
			return errors.Errorf("these models must first be upgraded to at least %v before upgrading the controller:\n -%s",
				requiredVersion, strings.Join(oldModels, "\n -"))
		}
	}

	return c.api.stateAccessor.SetModelAgentVersion(args.Version, args.IgnoreAgentVersions)
}

// CheckMongoStatusForUpgrade returns an error if the replicaset is not in a good
// enough state for an upgrade to continue. Exported for testing.
func (c *Client) CheckMongoStatusForUpgrade(session MongoSession) error {
	if skipReplicaCheck {
		// Skipping only occurs in tests where we need to avoid actually checking
		// the replicaset as tests don't run with this setting.
		return nil
	}
	replicaStatus, err := session.CurrentStatus()
	if err != nil {
		return errors.Annotate(err, "checking replicaset status")
	}

	// Iterate over the replicaset, and record any nodes that aren't either
	// primary or secondary.
	var notes []string
	for _, member := range replicaStatus.Members {
		switch member.State {
		case replicaset.PrimaryState:
			// All good.
		case replicaset.SecondaryState:
			// Also good.
		default:
			msg := fmt.Sprintf("node %d (%s) has state %s", member.Id, member.Address, member.State)
			notes = append(notes, msg)
		}
	}

	if len(notes) > 0 {
		return errors.Errorf("unable to upgrade, database %s", strings.Join(notes, ", "))
	}
	return nil
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.AbortCurrentUpgrade()
}

// FindTools returns a List containing all tools matching the given parameters.
func (c *Client) FindTools(args params.FindToolsParams) (result params.FindToolsResult, err error) {
	if err := c.checkCanWrite(); err != nil {
		return params.FindToolsResult{}, err
	}
	model, err := c.api.stateAccessor.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	result, err = c.api.toolsFinder.FindTools(args)
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

func (c *Client) toolVersionsForCAAS(args params.FindToolsParams, streamsVersions set.Strings, current version.Number) (params.FindToolsResult, error) {
	result := params.FindToolsResult{}
	controllerCfg, err := c.api.stateAccessor.ControllerConfig()
	if err != nil {
		return result, errors.Trace(err)
	}
	imageRepoDetails := controllerCfg.CAASImageRepo()
	if imageRepoDetails.Empty() {
		repoDetails, err := docker.NewImageRepoDetails(podcfg.JujudOCINamespace)
		if err != nil {
			return result, errors.Trace(err)
		}
		imageRepoDetails = *repoDetails
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
		if jujuversion.OfficialBuild == 0 && number.Build > 0 {
			continue
		}
		if args.MajorVersion != -1 && number.Major != args.MajorVersion {
			continue
		}
		number.Build = 0
		if !controllerCfg.Features().Contains(feature.DeveloperMode) && streamsVersions.Size() > 0 {
			if !streamsVersions.Contains(number.String()) {
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
		if err != nil {
			return result, errors.Annotatef(err, "cannot get architecture for %q:%q", imageName, number.String())
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

// Method was deprecated as of juju 2.9 and removed in juju 3.0. Please use the
// client/charms facade instead.
func (c *Client) AddCharm(args params.AddCharm) error {
	return errors.NewNotSupported(nil, "Deprecated AddCharm call has been removed in Juju 3.0; please use the charms facade instead")
}

// Method was deprecated as of juju 2.9 and removed in juju 3.0. Please use the
// client/charms facade instead.
func (c *Client) AddCharmWithAuthorization(args params.AddCharmWithAuthorization) error {
	return errors.NewNotSupported(nil, "Deprecated AddCharmWithAuthorization call has been removed in Juju 3.0; please use the charms facade instead")
}

// ResolveCharms resolves the best available charm URLs with series, for charm
// locations without a series specified.
//
// NOTE: ResolveCharms is deprecated as of juju 2.9 and charms facade version 3.
// Please discontinue use and move to the charms facade version.
//
// TODO: remove in juju 3.0
func (c *Client) ResolveCharms(args params.ResolveCharms) (params.ResolveCharmResults, error) {
	return params.ResolveCharmResults{}, errors.NewNotSupported(nil, "Deprecated ResolveChamrs call has been removed in Juju 3.0; please use the charms facade instead")
}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (c *Client) RetryProvisioning(p params.Entities) (params.ErrorResults, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(p.Entities)),
	}
	for i, e := range p.Entities {
		tag, err := names.ParseMachineTag(e.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := c.updateInstanceStatus(tag, map[string]interface{}{"transient": true}); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

type instanceStatus interface {
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(sInfo status.StatusInfo) error
}

func (c *Client) updateInstanceStatus(tag names.Tag, data map[string]interface{}) error {
	entity0, err := c.api.stateAccessor.FindEntity(tag)
	if err != nil {
		return err
	}
	statusGetterSetter, ok := entity0.(instanceStatus)
	if !ok {
		return apiservererrors.NotSupportedError(tag, "getting status")
	}
	existingStatusInfo, err := statusGetterSetter.InstanceStatus()
	if err != nil {
		return err
	}
	newData := existingStatusInfo.Data
	if newData == nil {
		newData = data
	} else {
		for k, v := range data {
			newData[k] = v
		}
	}
	if len(newData) > 0 && existingStatusInfo.Status != status.Error && existingStatusInfo.Status != status.ProvisioningError {
		return fmt.Errorf("%s is not in an error state (%v)", names.ReadableString(tag), existingStatusInfo.Status)
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  existingStatusInfo.Status,
		Message: existingStatusInfo.Message,
		Data:    newData,
		Since:   &now,
	}
	return statusGetterSetter.SetInstanceStatus(sInfo)
}

// APIHostPorts returns the API host/port addresses stored in state.
func (c *Client) APIHostPorts() (result params.APIHostPortsResult, err error) {
	if err := c.checkCanWrite(); err != nil {
		return result, err
	}

	ctrlSt := c.api.pool.SystemState()
	servers, err := ctrlSt.APIHostPortsForClients()
	if err != nil {
		return result, err
	}

	pServers := make([]network.HostPorts, len(servers))
	for i, hps := range servers {
		pServers[i] = hps.HostPorts()
	}

	result.Servers = params.FromHostsPorts(pServers)
	return result, nil
}

// CACert returns the certificate used to validate the state connection.
func (c *Client) CACert() (params.BytesResult, error) {
	cfg, err := c.api.stateAccessor.ControllerConfig()
	if err != nil {
		return params.BytesResult{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()
	return params.BytesResult{Result: []byte(caCert)}, nil
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
