// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/winrmprovisioner"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor Backend
	pool          Pool
	auth          facade.Authorizer
	resources     facade.Resources
	presence      facade.Presence

	client *Client
	// statusSetter provides common methods for updating an entity's provisioning status.
	statusSetter     *common.StatusSetter
	toolsFinder      *common.ToolsFinder
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
	// TODO(wallyworld) - we'll retain model config facade methods
	// on the client facade until GUI and Python client library are updated.
	*modelconfig.ModelConfigAPIV1

	api         *API
	newEnviron  func() (environs.BootstrapEnviron, error)
	check       *common.BlockChecker
	callContext context.ProviderCallContext
}

// ClientV1 serves the (v1) client-specific API methods.
type ClientV1 struct {
	*Client
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
		return common.ErrPerm
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
		return common.ErrPerm
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
		return common.ErrPerm
	}
	return nil
}

// NewFacade creates a version 1 Client facade to handle API requests.
func NewFacade(ctx facade.Context) (*Client, error) {
	return newFacade(ctx)
}

// NewFacadeV1 creates a version 1 Client facade to handle API requests.
func NewFacadeV1(ctx facade.Context) (*ClientV1, error) {
	client, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ClientV1{client}, nil
}

func newFacade(ctx facade.Context) (*Client, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()
	presence := ctx.Presence()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configGetter := stateenvirons.EnvironConfigGetter{State: st, Model: model}

	var newEnviron func() (environs.BootstrapEnviron, error)
	if model.Type() == state.ModelTypeCAAS {
		newEnviron = func() (environs.BootstrapEnviron, error) {
			f := stateenvirons.GetNewCAASBrokerFunc(caas.New)
			return f(st)
		}
	} else {
		newEnviron = func() (environs.BootstrapEnviron, error) {
			return environs.GetEnviron(configGetter, environs.New)
		}
	}

	modelUUID := model.UUID()

	urlGetter := common.NewToolsURLGetter(modelUUID, st)
	statusSetter := common.NewStatusSetter(st, common.AuthAlways())
	toolsFinder := common.NewToolsFinder(configGetter, st, urlGetter)
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
		&stateShim{st, model},
		&poolShim{ctx.StatePool()},
		&modelconfig.ModelConfigAPIV1{modelConfigAPI},
		resources,
		authorizer,
		presence,
		statusSetter,
		toolsFinder,
		newEnviron,
		blockChecker,
		state.CallContext(st),
		leadershipReader,
		modelCache,
	)
}

// NewClient creates a new instance of the Client Facade.
func NewClient(
	backend Backend,
	pool Pool,
	modelConfigAPI *modelconfig.ModelConfigAPIV1,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	statusSetter *common.StatusSetter,
	toolsFinder *common.ToolsFinder,
	newEnviron func() (environs.BootstrapEnviron, error),
	blockChecker *common.BlockChecker,
	callCtx context.ProviderCallContext,
	leadershipReader leadership.Reader,
	modelCache *cache.Model,
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	client := &Client{
		ModelConfigAPIV1: modelConfigAPI,
		api: &API{
			stateAccessor:    backend,
			pool:             pool,
			auth:             authorizer,
			resources:        resources,
			presence:         presence,
			statusSetter:     statusSetter,
			toolsFinder:      toolsFinder,
			leadershipReader: leadershipReader,
			modelCache:       modelCache,
		},
		newEnviron:  newEnviron,
		check:       blockChecker,
		callContext: callCtx,
	}
	return client, nil
}

// WatchAll initiates a watcher for entities in the connected model.
func (c *Client) WatchAll() (params.AllWatcherId, error) {
	if err := c.checkCanRead(); err != nil {
		return params.AllWatcherId{}, err
	}
	model, err := c.api.stateAccessor.Model()
	if err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := c.api.auth.GetAuthTag().(names.UserTag)
	isAdmin, err := common.HasModelAdmin(c.api.auth, apiUser, c.api.stateAccessor.ControllerTag(), model)
	if err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}
	watchParams := state.WatchParams{IncludeOffers: isAdmin}

	w := c.api.stateAccessor.Watch(watchParams)
	return params.AllWatcherId{
		AllWatcherId: c.api.resources.Register(w),
	}, nil
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
	if err := c.checkCanWrite(); err != nil {
		return params.AddMachinesResults{}, err
	}

	return c.AddMachinesV2(args)
}

// AddMachinesV2 adds new machines with the supplied parameters.
func (c *Client) AddMachinesV2(args params.AddMachines) (params.AddMachinesResults, error) {
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}
	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, p := range args.MachineParams {
		m, err := c.addOneMachine(p)
		results.Machines[i].Error = common.ServerError(err)
		if err == nil {
			results.Machines[i].Machine = m.Id()
		}
	}
	return results, nil
}

// InjectMachines injects a machine into state with provisioned status.
func (c *Client) InjectMachines(args params.AddMachines) (params.AddMachinesResults, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.AddMachinesResults{}, err
	}

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
	icfg, err := InstanceConfig(c.api.state(), args.MachineId, args.Nonce, args.DataDir)
	if err != nil {
		return result, common.ServerError(errors.Annotate(
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
		return result, common.ServerError(errors.Annotate(
			err, "getting model config",
		))
	} else {
		icfg.EnableOSUpgrade = cfg.EnableOSUpgrade()
		icfg.EnableOSRefreshUpdate = cfg.EnableOSRefreshUpdate()
	}

	osSeries, err := series.GetOSFromSeries(icfg.Series)
	if err != nil {
		return result, common.ServerError(errors.Annotatef(err,
			"cannot decide which provisioning script to generate based on this series %q", icfg.Series))
	}

	getProvisioningScript := sshprovisioner.ProvisioningScript
	if osSeries == os.Windows {
		getProvisioningScript = winrmprovisioner.ProvisioningScript
	}

	result.Script, err = getProvisioningScript(icfg)
	if err != nil {
		return result, common.ServerError(errors.Annotate(
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
		CloudTag:       names.NewCloudTag(model.Cloud()).String(),
		CloudRegion:    model.CloudRegion(),
		ProviderType:   conf.Type(),
		Name:           conf.Name(),
		Type:           string(model.Type()),
		UUID:           model.UUID(),
		OwnerTag:       model.Owner().String(),
		Life:           params.Life(model.Life().String()),
		ControllerUUID: state.ControllerTag().String(),
		IsController:   state.IsController(),
	}
	if agentVersion, exists := conf.AgentVersion(); exists {
		info.AgentVersion = &agentVersion
	}
	if tag, ok := model.CloudCredential(); ok {
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
			result.Error = common.ServerError(err)
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
	// Check k8s clusters.
	if env, ok := envOrBroker.(caas.ClusterMetadataChecker); ok {
		if _, err := env.GetClusterMetadata(""); err != nil {
			return errors.Annotate(err, "cannot make API call to provider")
		}
	}
	// If this is the controller model, also check to make sure that there are
	// no running migrations.  All models should have migration mode of None.
	if c.api.stateAccessor.IsController() {
		modelUUIDs, err := c.api.stateAccessor.AllModelUUIDs()
		if err != nil {
			return errors.Trace(err)
		}

		for _, modelUUID := range modelUUIDs {
			model, release, err := c.api.pool.GetModel(modelUUID)
			if err != nil {
				return errors.Trace(err)
			}
			if mode := model.MigrationMode(); mode != state.MigrationModeNone {
				release()
				return errors.Errorf("model \"%s/%s\" is %s, upgrade blocked", model.Owner().Name(), model.Name(), mode)
			}
			release()
		}
	}

	return c.api.stateAccessor.SetModelAgentVersion(args.Version, args.IgnoreAgentVersions)
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
func (c *Client) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.FindToolsResult{}, err
	}

	return c.api.toolsFinder.FindTools(args)
}

func (c *Client) AddCharm(args params.AddCharm) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	shim := application.NewStateShim(c.api.state())
	return application.AddCharmWithAuthorization(shim, params.AddCharmWithAuthorization{
		URL:     args.URL,
		Channel: args.Channel,
		Force:   args.Force,
	})
}

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func (c *Client) AddCharmWithAuthorization(args params.AddCharmWithAuthorization) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	shim := application.NewStateShim(c.api.state())
	return application.AddCharmWithAuthorization(shim, args)
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharms(args params.ResolveCharms) (params.ResolveCharmResults, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.ResolveCharmResults{}, err
	}

	shim := application.NewStateShim(c.api.state())
	return application.ResolveCharms(shim, args)
}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (c *Client) RetryProvisioning(p params.Entities) (params.ErrorResults, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	entityStatus := make([]params.EntityStatusArgs, len(p.Entities))
	for i, entity := range p.Entities {
		entityStatus[i] = params.EntityStatusArgs{Tag: entity.Tag, Data: map[string]interface{}{"transient": true}}
	}
	return c.api.statusSetter.UpdateStatus(params.SetStatus{
		Entities: entityStatus,
	})
}

// APIHostPorts returns the API host/port addresses stored in state.
func (c *Client) APIHostPorts() (result params.APIHostPortsResult, err error) {
	if err := c.checkCanWrite(); err != nil {
		return result, err
	}

	servers, err := c.api.stateAccessor.APIHostPortsForClients()
	if err != nil {
		return result, err
	}

	pServers := make([]network.ProviderHostPorts, len(servers))
	for i, hps := range servers {
		if pServers[i], err = hps.ToProviderHostPorts(c.api.stateAccessor); err != nil {
			return result, err
		}
	}

	result.Servers = params.FromProviderHostsPorts(pServers)
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

// FindTools returns a List containing all tools matching the given parameters.
func (c *ClientV1) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	if err := c.checkCanWrite(); err != nil {
		return params.FindToolsResult{}, err
	}

	if args.AgentStream != "" {
		return params.FindToolsResult{}, errors.New("requesting agent-stream not supported by model")
	}
	return c.api.toolsFinder.FindTools(args)
}
