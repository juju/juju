// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

func init() {
	common.RegisterStandardFacade("Client", 0, NewClient)
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
	compat
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
	urlGetter := common.NewToolsURLGetter(apiState.EnvironUUID(), apiState)
	client := &Client{
		api: &API{
			stateAccessor: apiState,
			auth:          authorizer,
			resources:     resources,
			statusSetter:  common.NewStatusSetter(st, common.AuthAlways()),
			toolsFinder:   common.NewToolsFinder(st, st, urlGetter),
		},
		check: common.NewBlockChecker(st)}
	// Plug in compatibility shims.
	client.compat = compat{client}
	return client, nil
}

func (c *Client) WatchAll() (params.AllWatcherId, error) {
	w := c.api.stateAccessor.Watch()
	return params.AllWatcherId{
		AllWatcherId: c.api.resources.Register(w),
	}, nil
}

// ServiceSet implements the server side of Client.ServiceSet. Values set to an
// empty string will be unset.
//
// (Deprecated) Use NewServiceSetForClientAPI instead, to preserve values set to
// an empty string, and use ServiceUnset to unset values.
func (c *Client) ServiceSet(p params.ServiceSet) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return service.ServiceSetSettingsStrings(svc, p.Options)
}

// NewServiceSetForClientAPI implements the server side of
// Client.NewServiceSetForClientAPI. This is exactly like ServiceSet except that
// it does not unset values that are set to an empty string.  ServiceUnset
// should be used for that.
//
// TODO(Nate): rename this to ServiceSet (and remove the deprecated ServiceSet)
// when the GUI handles the new behavior.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) NewServiceSetForClientAPI(p params.ServiceSet) error {
	svc, err := c.api.stateAccessor.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return newServiceSetSettingsStringsForClientAPI(svc, p.Options)
}

// ServiceUnset implements the server side of Client.ServiceUnset.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) ServiceUnset(p params.ServiceUnset) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(p.ServiceName)
	if err != nil {
		return err
	}
	settings := make(charm.Settings)
	for _, option := range p.Options {
		settings[option] = nil
	}
	return svc.UpdateConfigSettings(settings)
}

// ServiceCharmRelations implements the server side of Client.ServiceCharmRelations.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) ServiceCharmRelations(p params.ServiceCharmRelations) (params.ServiceCharmRelationsResults, error) {
	var results params.ServiceCharmRelationsResults
	service, err := c.api.stateAccessor.Service(p.ServiceName)
	if err != nil {
		return results, err
	}
	endpoints, err := service.Endpoints()
	if err != nil {
		return results, err
	}
	results.CharmRelations = make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		results.CharmRelations[i] = endpoint.Relation.Name
	}
	return results, nil
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

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) ServiceExpose(args params.ServiceExpose) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetExposed()
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) ServiceUnexpose(args params.ServiceUnexpose) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}

// ServiceDeploy fetches the charm from the charm store and deploys it.
// AddCharm or AddLocalCharm should be called to add the charm
// before calling ServiceDeploy, although for backward compatibility
// this is not necessary until 1.16 support is removed.
func (c *Client) ServiceDeploy(args params.ServiceDeploy) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return service.DeployService(c.api.state(), c.api.auth.GetAuthTag().String(), args)
}

// ServiceDeployWithNetworks works exactly like ServiceDeploy, but
// allows specifying networks to include or exclude on the machine
// where the charm gets deployed (either with args.Network or with
// constraints).
//
// TODO(dimitern): Drop the special handling of networks in favor of
// spaces constraints, once possible.
func (c *Client) ServiceDeployWithNetworks(args params.ServiceDeploy) error {
	return c.ServiceDeploy(args)
}

// newServiceSetSettingsStringsForClientAPI updates the settings for the given
// service, taking the configuration from a map of strings.
//
// TODO(Nate): replace serviceSetSettingsStrings with this onces the GUI no
// longer expects to be able to unset values by sending an empty string.
func newServiceSetSettingsStringsForClientAPI(service *state.Service, settings map[string]string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}

	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(settings)
	if err != nil {
		return err
	}

	return service.UpdateConfigSettings(changes)
}

// addServiceUnits adds a given number of units to a service.
func addServiceUnits(st *state.State, args params.AddServiceUnits) ([]*state.Unit, error) {
	service, err := st.Service(args.ServiceName)
	if err != nil {
		return nil, err
	}
	if args.NumUnits < 1 {
		return nil, fmt.Errorf("must add at least one unit")
	}

	// New API uses placement directives.
	if len(args.Placement) > 0 {
		return jjj.AddUnitsWithPlacement(st, service, args.NumUnits, args.Placement)
	}

	// Otherwise we use the older machine spec.
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
		return nil, fmt.Errorf("cannot use NumUnits with ToMachineSpec")
	}

	if args.ToMachineSpec != "" && names.IsValidMachine(args.ToMachineSpec) {
		_, err = st.Machine(args.ToMachineSpec)
		if err != nil {
			return nil, errors.Annotatef(err, `cannot add units for service "%v" to machine %v`, args.ServiceName, args.ToMachineSpec)
		}
	}
	return jjj.AddUnits(st, service, args.NumUnits, args.ToMachineSpec)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(args params.AddServiceUnits) (params.AddServiceUnitsResults, error) {
	return c.AddServiceUnitsWithPlacement(args)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnitsWithPlacement(args params.AddServiceUnits) (params.AddServiceUnitsResults, error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.AddServiceUnitsResults{}, errors.Trace(err)
	}
	units, err := addServiceUnits(c.api.state(), args)
	if err != nil {
		return params.AddServiceUnitsResults{}, err
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.String()
	}
	return params.AddServiceUnitsResults{Units: unitNames}, nil
}

// DestroyServiceUnits removes a given set of service units.
func (c *Client) DestroyServiceUnits(args params.DestroyServiceUnits) error {
	if err := c.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	var errs []string
	for _, name := range args.UnitNames {
		unit, err := c.api.stateAccessor.Unit(name)
		switch {
		case errors.IsNotFound(err):
			err = fmt.Errorf("unit %q does not exist", name)
		case err != nil:
		case unit.Life() != state.Alive:
			continue
		case unit.IsPrincipal():
			err = unit.Destroy()
		default:
			err = fmt.Errorf("unit %q is a subordinate", name)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("units", args.UnitNames, errs)
}

// ServiceDestroy destroys a given service.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) ServiceDestroy(args params.ServiceDestroy) error {
	if err := c.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.Destroy()
}

// GetServiceConstraints returns the constraints for a given service.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) GetServiceConstraints(args params.GetServiceConstraints) (params.GetConstraintsResults, error) {
	svc, err := c.api.stateAccessor.Service(args.ServiceName)
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	cons, err := svc.Constraints()
	return params.GetConstraintsResults{cons}, err
}

// GetEnvironmentConstraints returns the constraints for the environment.
func (c *Client) GetEnvironmentConstraints() (params.GetConstraintsResults, error) {
	cons, err := c.api.stateAccessor.EnvironConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{cons}, nil
}

// SetServiceConstraints sets the constraints for a given service.
// TODO(mattyw, all): This api call should be move to the new service facade. The client api version will then need bumping.
func (c *Client) SetServiceConstraints(args params.SetConstraints) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := c.api.stateAccessor.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetConstraints(args.Constraints)
}

// SetEnvironmentConstraints sets the constraints for the environment.
func (c *Client) SetEnvironmentConstraints(args params.SetConstraints) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.SetEnvironConstraints(args.Constraints)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	inEps, err := c.api.stateAccessor.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	rel, err := c.api.stateAccessor.AddRelation(inEps...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	outEps := make(map[string]charm.Relation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ServiceName)
		if err != nil {
			return params.AddRelationResults{}, err
		}
		outEps[inEp.ServiceName] = outEp.Relation
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(args params.DestroyRelation) error {
	if err := c.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	eps, err := c.api.stateAccessor.InferEndpoints(args.Endpoints...)
	if err != nil {
		return err
	}
	rel, err := c.api.stateAccessor.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
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
		conf, err := c.api.stateAccessor.EnvironConfig()
		if err != nil {
			return nil, err
		}
		p.Series = config.PreferredSeries(conf)
	}

	var placementDirective string
	if p.Placement != nil {
		env, err := c.api.stateAccessor.Environment()
		if err != nil {
			return nil, err
		}
		// For 1.21 we should support both UUID and name, and with 1.22
		// just support UUID
		if p.Placement.Scope != env.Name() && p.Placement.Scope != env.UUID() {
			return nil, fmt.Errorf("invalid environment name %q", p.Placement.Scope)
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
		Addresses:               params.NetworkAddresses(p.Addrs),
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
		return result, err
	}

	// Until DisablePackageCommands is retired, for backwards
	// compatibility, we must respect the client's request and
	// override any environment settings the user may have specified.
	// If the client does specify this setting, it will only ever be
	// true. False indicates the client doesn't care and we should use
	// what's specified in the environments.yaml file.
	if args.DisablePackageCommands {
		icfg.EnableOSRefreshUpdate = false
		icfg.EnableOSUpgrade = false
	} else if cfg, err := c.api.stateAccessor.EnvironConfig(); err != nil {
		return result, err
	} else {
		icfg.EnableOSUpgrade = cfg.EnableOSUpgrade()
		icfg.EnableOSRefreshUpdate = cfg.EnableOSRefreshUpdate()
	}

	result.Script, err = manual.ProvisioningScript(icfg)
	return result, err
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	var errs []string
	for _, id := range args.MachineNames {
		machine, err := c.api.stateAccessor.Machine(id)
		switch {
		case errors.IsNotFound(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case args.Force:
			err = machine.ForceDestroy()
		case machine.Life() != state.Alive:
			continue
		default:
			{
				if err := c.check.RemoveAllowed(); err != nil {
					return errors.Trace(err)
				}
				err = machine.Destroy()
			}
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("machines", args.MachineNames, errs)
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

// EnvironmentInfo returns information about the current environment (default
// series and type).
func (c *Client) EnvironmentInfo() (api.EnvironmentInfo, error) {
	state := c.api.stateAccessor
	conf, err := state.EnvironConfig()
	if err != nil {
		return api.EnvironmentInfo{}, err
	}
	env, err := state.Environment()
	if err != nil {
		return api.EnvironmentInfo{}, err
	}

	info := api.EnvironmentInfo{
		DefaultSeries: config.PreferredSeries(conf),
		ProviderType:  conf.Type(),
		Name:          conf.Name(),
		UUID:          env.UUID(),
		ServerUUID:    env.ServerUUID(),
	}
	return info, nil
}

// ShareEnvironment manages allowing and denying the given user(s) access to the environment.
func (c *Client) ShareEnvironment(args params.ModifyEnvironUsers) (result params.ErrorResults, err error) {
	var createdBy names.UserTag
	var ok bool
	if createdBy, ok = c.api.auth.GetAuthTag().(names.UserTag); !ok {
		return result, errors.Errorf("api connection is not through a user")
	}

	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		userTagString := arg.UserTag
		user, err := names.ParseUserTag(userTagString)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not share environment"))
			continue
		}
		switch arg.Action {
		case params.AddEnvUser:
			_, err := c.api.stateAccessor.AddEnvironmentUser(user, createdBy, "")
			if err != nil {
				err = errors.Annotate(err, "could not share environment")
				result.Results[i].Error = common.ServerError(err)
			}
		case params.RemoveEnvUser:
			err := c.api.stateAccessor.RemoveEnvironmentUser(user)
			if err != nil {
				err = errors.Annotate(err, "could not unshare environment")
				result.Results[i].Error = common.ServerError(err)
			}
		default:
			result.Results[i].Error = common.ServerError(errors.Errorf("unknown action %q", arg.Action))
		}
	}
	return result, nil
}

// EnvUserInfo returns information on all users in the environment.
func (c *Client) EnvUserInfo() (params.EnvUserInfoResults, error) {
	var results params.EnvUserInfoResults
	env, err := c.api.stateAccessor.Environment()
	if err != nil {
		return results, errors.Trace(err)
	}
	users, err := env.Users()
	if err != nil {
		return results, errors.Trace(err)
	}

	for _, user := range users {
		var lastConn *time.Time
		userLastConn, err := user.LastConnection()
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return results, errors.Trace(err)
			}
		} else {
			lastConn = &userLastConn
		}
		results.Results = append(results.Results, params.EnvUserInfoResult{
			Result: &params.EnvUserInfo{
				UserName:       user.UserName(),
				DisplayName:    user.DisplayName(),
				CreatedBy:      user.CreatedBy(),
				DateCreated:    user.DateCreated(),
				LastConnection: lastConn,
			},
		})
	}
	return results, nil
}

// GetAnnotations returns annotations about a given entity.
// This API is now deprecated - "Annotations" client should be used instead.
// TODO(anastasiamac) remove for Juju 2.x
func (c *Client) GetAnnotations(args params.GetAnnotations) (params.GetAnnotationsResults, error) {
	nothing := params.GetAnnotationsResults{}
	tag, err := c.parseEntityTag(args.Tag)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	entity, err := c.findEntity(tag)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	ann, err := c.api.stateAccessor.Annotations(entity)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	return params.GetAnnotationsResults{Annotations: ann}, nil
}

func (c *Client) parseEntityTag(tag0 string) (names.Tag, error) {
	tag, err := names.ParseTag(tag0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if tag.Kind() == names.CharmTagKind {
		return nil, common.NotSupportedError(tag, "client.annotations")
	}
	return tag, nil
}

func (c *Client) findEntity(tag names.Tag) (state.GlobalEntity, error) {
	entity0, err := c.api.stateAccessor.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.GlobalEntity)
	if !ok {
		return nil, common.NotSupportedError(tag, "annotations")
	}
	return entity, nil
}

// SetAnnotations stores annotations about a given entity.
// This API is now deprecated - "Annotations" client should be used instead.
// TODO(anastasiamac) remove for Juju 2.x
func (c *Client) SetAnnotations(args params.SetAnnotations) error {
	tag, err := c.parseEntityTag(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	entity, err := c.findEntity(tag)
	if err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.SetAnnotations(entity, args.Pairs)
}

// AgentVersion returns the current version that the API server is running.
func (c *Client) AgentVersion() (params.AgentVersionResult, error) {
	return params.AgentVersionResult{Version: version.Current}, nil
}

// EnvironmentGet implements the server-side part of the
// get-environment CLI command.
func (c *Client) EnvironmentGet() (params.EnvironmentConfigResults, error) {
	result := params.EnvironmentConfigResults{}
	// Get the existing environment config from the state.
	config, err := c.api.stateAccessor.EnvironConfig()
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}

// EnvironmentSet implements the server-side part of the
// set-environment CLI command.
func (c *Client) EnvironmentSet(args params.EnvironmentSet) error {
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
	return c.api.stateAccessor.UpdateEnvironConfig(attrs, nil, checkAgentVersion)
}

// EnvironmentUnset implements the server-side part of the
// set-environment CLI command.
func (c *Client) EnvironmentUnset(args params.EnvironmentUnset) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// TODO(waigani) 2014-3-11 #1167616
	// Add a txn retry loop to ensure that the settings on disk have not
	// changed underneath us.
	return c.api.stateAccessor.UpdateEnvironConfig(nil, args.Keys, nil)
}

// SetEnvironAgentVersion sets the environment agent version.
func (c *Client) SetEnvironAgentVersion(args params.SetEnvironAgentVersion) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.api.stateAccessor.SetEnvironAgentVersion(args.Version)
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

func destroyErr(desc string, ids, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	msg := "some %s were not destroyed"
	if len(errs) == len(ids) {
		msg = "no %s were destroyed"
	}
	msg = fmt.Sprintf(msg, desc)
	return fmt.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}

func (c *Client) AddCharm(args params.CharmURL) error {
	return service.AddCharmWithAuthorization(c.api.state(), params.AddCharmWithAuthorization{
		URL: args.URL,
	})
}

// AddCharmWithAuthorization adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func (c *Client) AddCharmWithAuthorization(args params.AddCharmWithAuthorization) error {
	return service.AddCharmWithAuthorization(c.api.state(), args)
}

// ResolveCharm resolves the best available charm URLs with series, for charm
// locations without a series specified.
func (c *Client) ResolveCharms(args params.ResolveCharms) (params.ResolveCharmResults, error) {
	return service.ResolveCharms(c.api.state(), args)
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

// EnsureAvailability ensures the availability of Juju state servers.
// DEPRECATED: remove when we stop supporting 1.20 and earlier clients.
// This API is now on the HighAvailability facade.
func (c *Client) EnsureAvailability(args params.StateServersSpecs) (params.StateServersChangeResults, error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.StateServersChangeResults{}, errors.Trace(err)
	}
	results := params.StateServersChangeResults{Results: make([]params.StateServersChangeResult, len(args.Specs))}
	for i, stateServersSpec := range args.Specs {
		result, err := highavailability.EnsureAvailabilitySingle(c.api.state(), stateServersSpec)
		results.Results[i].Result = result
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// DestroyEnvironment will try to destroy the current environment.
// If there is a block on destruction, this method will return an error.
func (c *Client) DestroyEnvironment() (err error) {
	if err := c.check.DestroyAllowed(); err != nil {
		return errors.Trace(err)
	}

	environTag := c.api.stateAccessor.EnvironTag()
	return errors.Trace(common.DestroyEnvironment(c.api.state(), environTag))
}
