// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

func init() {
	common.RegisterStandardFacade("Client", 1, NewClient)
}

var logger = loggo.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor stateInterface
	auth          common.Authorizer
	resources     *common.Resources
	client        *Client
	// statusSetter provides common methods for updating an entity's provisioning status.
	statusSetter *common.StatusSetter
	toolsFinder  *common.ToolsFinder
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
	api   *API
	check *common.BlockChecker
}

var getState = func(st *state.State) stateInterface {
	return &stateShim{st}
}

// NewClient creates a new instance of the Client Facade.
func NewClient(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	apiState := getState(st)
	urlGetter := common.NewToolsURLGetter(apiState.ModelUUID(), apiState)
	client := &Client{
		api: &API{
			stateAccessor: apiState,
			auth:          authorizer,
			resources:     resources,
			statusSetter:  common.NewStatusSetter(st, common.AuthAlways()),
			toolsFinder:   common.NewToolsFinder(st, st, urlGetter),
		},
		check: common.NewBlockChecker(st)}
	return client, nil
}

func (c *Client) WatchAll() (params.AllWatcherId, error) {
	w := c.api.stateAccessor.Watch()
	return params.AllWatcherId{
		AllWatcherId: c.api.resources.Register(w),
	}, nil
}

// Resolved implements the server side of Client.Resolved.
func (c *Client) Resolved(p params.Resolved) error {
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
	cons, err := c.api.stateAccessor.ModelConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *Client) SetModelConstraints(args params.SetConstraints) error {
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
		env, err := c.api.stateAccessor.Model()
		if err != nil {
			return nil, err
		}
		// For 1.21 we should support both UUID and name, and with 1.22
		// just support UUID
		if p.Placement.Scope != env.Name() && p.Placement.Scope != env.UUID() {
			return nil, fmt.Errorf("invalid model name %q", p.Placement.Scope)
		}
		placementDirective = p.Placement.Directive
	}

	jobs, err := common.StateJobs(p.Jobs)
	if err != nil {
		return nil, err
	}
	template := state.MachineTemplate{
		Series:      p.Series,
		Constraints: p.Constraints,
		InstanceId:  p.InstanceId,
		Jobs:        jobs,
		Nonce:       p.Nonce,
		HardwareCharacteristics: p.HardwareCharacteristics,
		Addresses:               params.NetworkAddresses(p.Addrs...),
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

	result.Script, err = manual.ProvisioningScript(icfg)
	if err != nil {
		return result, common.ServerError(errors.Annotate(
			err, "getting provisioning script",
		))
	}
	return result, nil
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	if err := c.check.RemoveAllowed(); !args.Force && err != nil {
		return errors.Trace(err)
	}

	return common.DestroyMachines(c.api.stateAccessor, args.Force, args.MachineNames...)
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(args params.CharmInfo) (api.CharmInfo, error) {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return api.CharmInfo{}, err
	}
	charm, err := c.api.stateAccessor.Charm(curl)
	if err != nil {
		return api.CharmInfo{}, err
	}
	info := api.CharmInfo{
		Revision: charm.Revision(),
		URL:      curl.String(),
		Config:   charm.Config(),
		Meta:     charm.Meta(),
		Actions:  charm.Actions(),
	}
	return info, nil
}

// ModelInfo returns information about the current model (default
// series and type).
func (c *Client) ModelInfo() (params.ModelInfo, error) {
	state := c.api.stateAccessor
	conf, err := state.ModelConfig()
	if err != nil {
		return params.ModelInfo{}, err
	}
	env, err := state.Model()
	if err != nil {
		return params.ModelInfo{}, err
	}

	info := params.ModelInfo{
		DefaultSeries:  config.PreferredSeries(conf),
		ProviderType:   conf.Type(),
		Name:           conf.Name(),
		UUID:           env.UUID(),
		ControllerUUID: env.ControllerUUID(),
	}
	return info, nil
}

// ModelUserInfo returns information on all users in the model.
func (c *Client) ModelUserInfo() (params.ModelUserInfoResults, error) {
	var results params.ModelUserInfoResults
	env, err := c.api.stateAccessor.Model()
	if err != nil {
		return results, errors.Trace(err)
	}
	users, err := env.Users()
	if err != nil {
		return results, errors.Trace(err)
	}

	for _, user := range users {
		var result params.ModelUserInfoResult
		userInfo, err := common.ModelUserInfo(user)
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
	return params.AgentVersionResult{Version: jujuversion.Current}, nil
}

// ModelGet implements the server-side part of the
// get-model-config CLI command.
func (c *Client) ModelGet() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	// Get the existing environment config from the state.
	config, err := c.api.stateAccessor.ModelConfig()
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}

// ModelSet implements the server-side part of the
// set-model-config CLI command.
func (c *Client) ModelSet(args params.ModelSet) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	checkAgentVersion := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if v, found := updateAttrs["agent-version"]; found {
			oldVersion, _ := oldConfig.AgentVersion()
			if v != oldVersion.String() {
				return fmt.Errorf("agent-version cannot be changed")
			}
		}
		return nil
	}
	// Replace any deprecated attributes with their new values.
	attrs := config.ProcessDeprecatedAttributes(args.Config)
	// TODO(waigani) 2014-3-11 #1167616
	// Add a txn retry loop to ensure that the settings on disk have not
	// changed underneath us.
	return c.api.stateAccessor.UpdateModelConfig(attrs, nil, checkAgentVersion)
}

// ModelUnset implements the server-side part of the
// set-model-config CLI command.
func (c *Client) ModelUnset(args params.ModelUnset) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// TODO(waigani) 2014-3-11 #1167616
	// Add a txn retry loop to ensure that the settings on disk have not
	// changed underneath us.
	return c.api.stateAccessor.UpdateModelConfig(nil, args.Keys, nil)
}

// SetModelAgentVersion sets the model agent version.
func (c *Client) SetModelAgentVersion(args params.SetModelAgentVersion) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Before changing the agent version to trigger an upgrade or downgrade,
	// we'll do a very basic check to ensure the
	cfg, err := c.api.stateAccessor.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}
	env, err := getEnvironment(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	if err := environs.CheckProviderAPI(env); err != nil {
		return err
	}
	return c.api.stateAccessor.SetModelAgentVersion(args.Version)
}

var getEnvironment = func(cfg *config.Config) (environs.Environ, error) {
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.AbortCurrentUpgrade()
}

// FindTools returns a List containing all tools matching the given parameters.
func (c *Client) FindTools(args params.FindToolsParams) (params.FindToolsResult, error) {
	return c.api.toolsFinder.FindTools(args)
}

func (c *Client) AddCharm(args params.AddCharm) error {
	return application.AddCharmWithAuthorization(c.api.state(), params.AddCharmWithAuthorization{
		URL:     args.URL,
		Channel: args.Channel,
	})
}

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func (c *Client) AddCharmWithAuthorization(args params.AddCharmWithAuthorization) error {
	return application.AddCharmWithAuthorization(c.api.state(), args)
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharms(args params.ResolveCharms) (params.ResolveCharmResults, error) {
	return application.ResolveCharms(c.api.state(), args)
}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (c *Client) RetryProvisioning(p params.Entities) (params.ErrorResults, error) {
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
	var servers [][]network.HostPort
	if servers, err = c.api.stateAccessor.APIHostPorts(); err != nil {
		return params.APIHostPortsResult{}, err
	}
	result.Servers = params.FromNetworkHostsPorts(servers)
	return result, nil
}

// DestroyModel will try to destroy the current model.
// If there is a block on destruction, this method will return an error.
func (c *Client) DestroyModel() (err error) {
	if err := c.check.DestroyAllowed(); err != nil {
		return errors.Trace(err)
	}

	modelTag := c.api.stateAccessor.ModelTag()
	return errors.Trace(common.DestroyModel(c.api.state(), modelTag))
}
