// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

func init() {
	common.RegisterStandardFacade("Client", 0, NewClient)
}

var (
	logger = loggo.GetLogger("juju.apiserver.client")

	stateStorage = (*state.State).Storage
)

type API struct {
	state     *state.State
	auth      common.Authorizer
	resources *common.Resources
	client    *Client
	// statusSetter provides common methods for updating an entity's provisioning status.
	statusSetter *common.StatusSetter
	toolsFinder  *common.ToolsFinder
}

// Client serves client-specific API methods.
type Client struct {
	api *API
}

// NewClient creates a new instance of the Client Facade.
func NewClient(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	env, err := st.Environment()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(env.UUID(), st)
	return &Client{api: &API{
		state:        st,
		auth:         authorizer,
		resources:    resources,
		statusSetter: common.NewStatusSetter(st, common.AuthAlways()),
		toolsFinder:  common.NewToolsFinder(st, st, urlGetter),
	}}, nil
}

func (c *Client) WatchAll() (params.AllWatcherId, error) {
	w := c.api.state.Watch()
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
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return serviceSetSettingsStrings(svc, p.Options)
}

// NewServiceSetForClientAPI implements the server side of
// Client.NewServiceSetForClientAPI. This is exactly like ServiceSet except that
// it does not unset values that are set to an empty string.  ServiceUnset
// should be used for that.
//
// TODO(Nate): rename this to ServiceSet (and remove the deprecated ServiceSet)
// when the GUI handles the new behavior.
func (c *Client) NewServiceSetForClientAPI(p params.ServiceSet) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return newServiceSetSettingsStringsForClientAPI(svc, p.Options)
}

// ServiceUnset implements the server side of Client.ServiceUnset.
func (c *Client) ServiceUnset(p params.ServiceUnset) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	settings := make(charm.Settings)
	for _, option := range p.Options {
		settings[option] = nil
	}
	return svc.UpdateConfigSettings(settings)
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *Client) ServiceSetYAML(p params.ServiceSetYAML) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return serviceSetSettingsYAML(svc, p.Config)
}

// ServiceCharmRelations implements the server side of Client.ServiceCharmRelations.
func (c *Client) ServiceCharmRelations(p params.ServiceCharmRelations) (params.ServiceCharmRelationsResults, error) {
	var results params.ServiceCharmRelationsResults
	service, err := c.api.state.Service(p.ServiceName)
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
	unit, err := c.api.state.Unit(p.UnitName)
	if err != nil {
		return err
	}
	return unit.Resolve(p.Retry)
}

// PublicAddress implements the server side of Client.PublicAddress.
func (c *Client) PublicAddress(p params.PublicAddress) (results params.PublicAddressResults, err error) {
	switch {
	case names.IsValidMachine(p.Target):
		machine, err := c.api.state.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr := network.SelectPublicAddress(machine.Addresses())
		if addr == "" {
			return results, fmt.Errorf("machine %q has no public address", machine)
		}
		return params.PublicAddressResults{PublicAddress: addr}, nil

	case names.IsValidUnit(p.Target):
		unit, err := c.api.state.Unit(p.Target)
		if err != nil {
			return results, err
		}
		addr, ok := unit.PublicAddress()
		if !ok {
			return results, fmt.Errorf("unit %q has no public address", unit)
		}
		return params.PublicAddressResults{PublicAddress: addr}, nil
	}
	return results, fmt.Errorf("unknown unit or machine %q", p.Target)
}

// PrivateAddress implements the server side of Client.PrivateAddress.
func (c *Client) PrivateAddress(p params.PrivateAddress) (results params.PrivateAddressResults, err error) {
	switch {
	case names.IsValidMachine(p.Target):
		machine, err := c.api.state.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr := network.SelectInternalAddress(machine.Addresses(), false)
		if addr == "" {
			return results, fmt.Errorf("machine %q has no internal address", machine)
		}
		return params.PrivateAddressResults{PrivateAddress: addr}, nil

	case names.IsValidUnit(p.Target):
		unit, err := c.api.state.Unit(p.Target)
		if err != nil {
			return results, err
		}
		addr, ok := unit.PrivateAddress()
		if !ok {
			return results, fmt.Errorf("unit %q has no internal address", unit)
		}
		return params.PrivateAddressResults{PrivateAddress: addr}, nil
	}
	return results, fmt.Errorf("unknown unit or machine %q", p.Target)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(args params.ServiceExpose) error {
	svc, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetExposed()
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(args params.ServiceUnexpose) error {
	svc, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}

var CharmStore charm.Repository = charm.Store

func networkTagsToNames(tags []string) ([]string, error) {
	netNames := make([]string, len(tags))
	for i, tag := range tags {
		t, err := names.ParseNetworkTag(tag)
		if err != nil {
			return nil, err
		}
		netNames[i] = t.Id()
	}
	return netNames, nil
}

// ServiceDeploy fetches the charm from the charm store and deploys it.
// AddCharm or AddLocalCharm should be called to add the charm
// before calling ServiceDeploy, although for backward compatibility
// this is not necessary until 1.16 support is removed.
func (c *Client) ServiceDeploy(args params.ServiceDeploy) error {
	curl, err := charm.ParseURL(args.CharmUrl)
	if err != nil {
		return err
	}
	if curl.Revision < 0 {
		return fmt.Errorf("charm url must include revision")
	}

	if args.ToMachineSpec != "" && names.IsValidMachine(args.ToMachineSpec) {
		_, err = c.api.state.Machine(args.ToMachineSpec)
		if err != nil {
			return fmt.Errorf(`cannot deploy "%v" to machine %v: %v`, args.ServiceName, args.ToMachineSpec, err)
		}
	}

	// Try to find the charm URL in state first.
	ch, err := c.api.state.Charm(curl)
	if errors.IsNotFound(err) {
		// Remove this whole if block when 1.16 compatibility is dropped.
		if curl.Schema != "cs" {
			return fmt.Errorf(`charm url has unsupported schema %q`, curl.Schema)
		}
		err = c.AddCharm(params.CharmURL{args.CharmUrl})
		if err != nil {
			return err
		}
		ch, err = c.api.state.Charm(curl)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var settings charm.Settings
	if len(args.ConfigYAML) > 0 {
		settings, err = ch.Config().ParseSettingsYAML([]byte(args.ConfigYAML), args.ServiceName)
	} else if len(args.Config) > 0 {
		// Parse config in a compatile way (see function comment).
		settings, err = parseSettingsCompatible(ch, args.Config)
	}
	if err != nil {
		return err
	}
	// Convert network tags to names for any given networks.
	requestedNetworks, err := networkTagsToNames(args.Networks)
	if err != nil {
		return err
	}

	_, err = juju.DeployService(c.api.state,
		juju.DeployServiceParams{
			ServiceName: args.ServiceName,
			// TODO(dfc) ServiceOwner should be a tag
			ServiceOwner:   c.api.auth.GetAuthTag().String(),
			Charm:          ch,
			NumUnits:       args.NumUnits,
			ConfigSettings: settings,
			Constraints:    args.Constraints,
			ToMachineSpec:  args.ToMachineSpec,
			Networks:       requestedNetworks,
		})
	return err
}

// ServiceDeployWithNetworks works exactly like ServiceDeploy, but
// allows specifying networks to include or exclude on the machine
// where the charm gets deployed (either with args.Network or with
// constraints).
func (c *Client) ServiceDeployWithNetworks(args params.ServiceDeploy) error {
	return c.ServiceDeploy(args)
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// All parameters in params.ServiceUpdate except the service name are optional.
func (c *Client) ServiceUpdate(args params.ServiceUpdate) error {
	service, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	// Set the charm for the given service.
	if args.CharmUrl != "" {
		if err = c.serviceSetCharm(service, args.CharmUrl, args.ForceCharmUrl); err != nil {
			return err
		}
	}
	// Set the minimum number of units for the given service.
	if args.MinUnits != nil {
		if err = service.SetMinUnits(*args.MinUnits); err != nil {
			return err
		}
	}
	// Set up service's settings.
	if args.SettingsYAML != "" {
		if err = serviceSetSettingsYAML(service, args.SettingsYAML); err != nil {
			return err
		}
	} else if len(args.SettingsStrings) > 0 {
		if err = serviceSetSettingsStrings(service, args.SettingsStrings); err != nil {
			return err
		}
	}
	// Update service's constraints.
	if args.Constraints != nil {
		return service.SetConstraints(*args.Constraints)
	}
	return nil
}

// serviceSetCharm sets the charm for the given service.
func (c *Client) serviceSetCharm(service *state.Service, url string, force bool) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return err
	}
	sch, err := c.api.state.Charm(curl)
	if errors.IsNotFound(err) {
		// Charms should be added before trying to use them, with
		// AddCharm or AddLocalCharm API calls. When they're not,
		// we're reverting to 1.16 compatibility mode.
		return c.serviceSetCharm1dot16(service, curl, force)
	}
	if err != nil {
		return err
	}
	return service.SetCharm(sch, force)
}

// serviceSetCharm1dot16 sets the charm for the given service in 1.16
// compatibility mode. Remove this when support for 1.16 is dropped.
func (c *Client) serviceSetCharm1dot16(service *state.Service, curl *charm.URL, force bool) error {
	if curl.Schema != "cs" {
		return fmt.Errorf(`charm url has unsupported schema %q`, curl.Schema)
	}
	if curl.Revision < 0 {
		return fmt.Errorf("charm url must include revision")
	}
	err := c.AddCharm(params.CharmURL{curl.String()})
	if err != nil {
		return err
	}
	ch, err := c.api.state.Charm(curl)
	if err != nil {
		return err
	}
	return service.SetCharm(ch, force)
}

// serviceSetSettingsYAML updates the settings for the given service,
// taking the configuration from a YAML string.
func serviceSetSettingsYAML(service *state.Service, settings string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}
	changes, err := ch.Config().ParseSettingsYAML([]byte(settings), service.Name())
	if err != nil {
		return err
	}
	return service.UpdateConfigSettings(changes)
}

// serviceSetSettingsStrings updates the settings for the given service,
// taking the configuration from a map of strings.
func serviceSetSettingsStrings(service *state.Service, settings map[string]string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch, settings)
	if err != nil {
		return err
	}
	return service.UpdateConfigSettings(changes)
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

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(args params.ServiceSetCharm) error {
	service, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return c.serviceSetCharm(service, args.CharmUrl, args.Force)
}

// addServiceUnits adds a given number of units to a service.
func addServiceUnits(state *state.State, args params.AddServiceUnits) ([]*state.Unit, error) {
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return nil, err
	}
	if args.NumUnits < 1 {
		return nil, fmt.Errorf("must add at least one unit")
	}
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
		return nil, fmt.Errorf("cannot use NumUnits with ToMachineSpec")
	}
	return juju.AddUnits(state, service, args.NumUnits, args.ToMachineSpec)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(args params.AddServiceUnits) (params.AddServiceUnitsResults, error) {
	units, err := addServiceUnits(c.api.state, args)
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
	var errs []string
	for _, name := range args.UnitNames {
		unit, err := c.api.state.Unit(name)
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
func (c *Client) ServiceDestroy(args params.ServiceDestroy) error {
	svc, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.Destroy()
}

// GetServiceConstraints returns the constraints for a given service.
func (c *Client) GetServiceConstraints(args params.GetServiceConstraints) (params.GetConstraintsResults, error) {
	svc, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	cons, err := svc.Constraints()
	return params.GetConstraintsResults{cons}, err
}

// GetEnvironmentConstraints returns the constraints for the environment.
func (c *Client) GetEnvironmentConstraints() (params.GetConstraintsResults, error) {
	cons, err := c.api.state.EnvironConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{cons}, nil
}

// SetServiceConstraints sets the constraints for a given service.
func (c *Client) SetServiceConstraints(args params.SetConstraints) error {
	svc, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetConstraints(args.Constraints)
}

// SetEnvironmentConstraints sets the constraints for the environment.
func (c *Client) SetEnvironmentConstraints(args params.SetConstraints) error {
	return c.api.state.SetEnvironConstraints(args.Constraints)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	inEps, err := c.api.state.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	rel, err := c.api.state.AddRelation(inEps...)
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
	eps, err := c.api.state.InferEndpoints(args.Endpoints...)
	if err != nil {
		return err
	}
	rel, err := c.api.state.EndpointsRelation(eps...)
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
		conf, err := c.api.state.EnvironConfig()
		if err != nil {
			return nil, err
		}
		p.Series = config.PreferredSeries(conf)
	}

	var placementDirective string
	if p.Placement != nil {
		env, err := c.api.state.Environment()
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

	jobs, err := stateJobs(p.Jobs)
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
		Addresses:               p.Addrs,
		Placement:               placementDirective,
	}
	if p.ContainerType == "" {
		return c.api.state.AddOneMachine(template)
	}
	if p.ParentId != "" {
		return c.api.state.AddMachineInsideMachine(template, p.ParentId, p.ContainerType)
	}
	return c.api.state.AddMachineInsideNewMachine(template, template, p.ContainerType)
}

func stateJobs(jobs []params.MachineJob) ([]state.MachineJob, error) {
	newJobs := make([]state.MachineJob, len(jobs))
	for i, job := range jobs {
		newJob, err := machineJobFromParams(job)
		if err != nil {
			return nil, err
		}
		newJobs[i] = newJob
	}
	return newJobs, nil
}

// machineJobFromParams returns the job corresponding to params.MachineJob.
// TODO(dfc) this function should live in apiserver/params, move there once
// state does not depend on apiserver/params
func machineJobFromParams(job params.MachineJob) (state.MachineJob, error) {
	switch job {
	case params.JobHostUnits:
		return state.JobHostUnits, nil
	case params.JobManageEnviron:
		return state.JobManageEnviron, nil
	case params.JobManageNetworking:
		return state.JobManageNetworking, nil
	case params.JobManageStateDeprecated:
		// Deprecated in 1.18.
		return state.JobManageStateDeprecated, nil
	default:
		return -1, errors.Errorf("invalid machine job %q", job)
	}
}

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (params.ProvisioningScriptResult, error) {
	var result params.ProvisioningScriptResult
	mcfg, err := MachineConfig(c.api.state, args.MachineId, args.Nonce, args.DataDir)
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
		mcfg.EnableOSRefreshUpdate = false
		mcfg.EnableOSUpgrade = false
	} else if cfg, err := c.api.state.EnvironConfig(); err != nil {
		return result, err
	} else {
		mcfg.EnableOSUpgrade = cfg.EnableOSUpgrade()
		mcfg.EnableOSRefreshUpdate = cfg.EnableOSRefreshUpdate()
	}

	result.Script, err = manual.ProvisioningScript(mcfg)
	return result, err
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	var errs []string
	for _, id := range args.MachineNames {
		machine, err := c.api.state.Machine(id)
		switch {
		case errors.IsNotFound(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case args.Force:
			err = machine.ForceDestroy()
		case machine.Life() != state.Alive:
			continue
		default:
			err = machine.Destroy()
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
	charm, err := c.api.state.Charm(curl)
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
	state := c.api.state
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
	}
	return info, nil
}

// ShareEnvironment allows the given user(s) access to the environment.
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
			_, err := c.api.state.AddEnvironmentUser(user, createdBy)
			if err != nil {
				err = errors.Annotate(err, "could not share environment")
				result.Results[i].Error = common.ServerError(err)
			}
		case params.RemoveEnvUser:
			err := c.api.state.RemoveEnvironmentUser(user)
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

// GetAnnotations returns annotations about a given entity.
func (c *Client) GetAnnotations(args params.GetAnnotations) (params.GetAnnotationsResults, error) {
	nothing := params.GetAnnotationsResults{}
	tag, err := names.ParseTag(args.Tag)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	entity, err := c.findEntity(tag)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	ann, err := entity.Annotations()
	if err != nil {
		return nothing, errors.Trace(err)
	}
	return params.GetAnnotationsResults{Annotations: ann}, nil
}

func (c *Client) findEntity(tag names.Tag) (state.Annotator, error) {
	entity0, err := c.api.state.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.Annotator)
	if !ok {
		return nil, common.NotSupportedError(tag, "annotations")
	}
	return entity, nil
}

// SetAnnotations stores annotations about a given entity.
func (c *Client) SetAnnotations(args params.SetAnnotations) error {
	tag, err := names.ParseTag(args.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	entity, err := c.findEntity(tag)
	if err != nil {
		return errors.Trace(err)
	}
	return entity.SetAnnotations(args.Pairs)
}

// parseSettingsCompatible parses setting strings in a way that is
// compatible with the behavior before this CL based on the issue
// http://pad.lv/1194945. Until then setting an option to an empty
// string caused it to reset to the default value. We now allow
// empty strings as actual values, but we want to preserve the API
// behavior.
func parseSettingsCompatible(ch *state.Charm, settings map[string]string) (charm.Settings, error) {
	setSettings := map[string]string{}
	unsetSettings := charm.Settings{}
	// Split settings into those which set and those which unset a value.
	for name, value := range settings {
		if value == "" {
			unsetSettings[name] = nil
			continue
		}
		setSettings[name] = value
	}
	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(setSettings)
	if err != nil {
		return nil, err
	}
	// Validate the unsettings and merge them into the changes.
	unsetSettings, err = ch.Config().ValidateSettings(unsetSettings)
	if err != nil {
		return nil, err
	}
	for name := range unsetSettings {
		changes[name] = nil
	}
	return changes, nil
}

// AgentVersion returns the current version that the API server is running.
func (c *Client) AgentVersion() (params.AgentVersionResult, error) {
	return params.AgentVersionResult{Version: version.Current.Number}, nil
}

// EnvironmentGet implements the server-side part of the
// get-environment CLI command.
func (c *Client) EnvironmentGet() (params.EnvironmentGetResults, error) {
	result := params.EnvironmentGetResults{}
	// Get the existing environment config from the state.
	config, err := c.api.state.EnvironConfig()
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}

// EnvironmentSet implements the server-side part of the
// set-environment CLI command.
func (c *Client) EnvironmentSet(args params.EnvironmentSet) error {
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
	// TODO(waigani) 2014-3-11 #1167616
	// Add a txn retry loop to ensure that the settings on disk have not
	// changed underneath us.
	return c.api.state.UpdateEnvironConfig(args.Config, nil, checkAgentVersion)
}

// EnvironmentUnset implements the server-side part of the
// set-environment CLI command.
func (c *Client) EnvironmentUnset(args params.EnvironmentUnset) error {
	// TODO(waigani) 2014-3-11 #1167616
	// Add a txn retry loop to ensure that the settings on disk have not
	// changed underneath us.
	return c.api.state.UpdateEnvironConfig(nil, args.Keys, nil)
}

// SetEnvironAgentVersion sets the environment agent version.
func (c *Client) SetEnvironAgentVersion(args params.SetEnvironAgentVersion) error {
	return c.api.state.SetEnvironAgentVersion(args.Version)
}

// AbortCurrentUpgrade aborts and archives the current upgrade
// synchronisation record, if any.
func (c *Client) AbortCurrentUpgrade() error {
	return c.api.state.AbortCurrentUpgrade()
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

// AddCharm adds the given charm URL (which must include revision) to
// the environment, if it does not exist yet. Local charms are not
// supported, only charm store URLs. See also AddLocalCharm().
func (c *Client) AddCharm(args params.CharmURL) error {
	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return err
	}
	if charmURL.Schema != "cs" {
		return fmt.Errorf("only charm store charm URLs are supported, with cs: schema")
	}
	if charmURL.Revision < 0 {
		return fmt.Errorf("charm URL must include revision")
	}

	// First, check if a pending or a real charm exists in state.
	stateCharm, err := c.api.state.PrepareStoreCharmUpload(charmURL)
	if err == nil && stateCharm.IsUploaded() {
		// Charm already in state (it was uploaded already).
		return nil
	} else if err != nil {
		return err
	}

	// Get the charm and its information from the store.
	envConfig, err := c.api.state.EnvironConfig()
	if err != nil {
		return err
	}
	store := config.SpecializeCharmRepo(CharmStore, envConfig)
	downloadedCharm, err := store.Get(charmURL)
	if err != nil {
		return errors.Annotatef(err, "cannot download charm %q", charmURL.String())
	}

	// Open it and calculate the SHA256 hash.
	downloadedBundle, ok := downloadedCharm.(*charm.CharmArchive)
	if !ok {
		return errors.Errorf("expected a charm archive, got %T", downloadedCharm)
	}
	archive, err := os.Open(downloadedBundle.Path)
	if err != nil {
		return errors.Annotate(err, "cannot read downloaded charm")
	}
	defer archive.Close()
	bundleSHA256, size, err := utils.ReadSHA256(archive)
	if err != nil {
		return errors.Annotate(err, "cannot calculate SHA256 hash of charm")
	}
	if _, err := archive.Seek(0, 0); err != nil {
		return errors.Annotate(err, "cannot rewind charm archive")
	}

	// Store the charm archive in environment storage.
	return StoreCharmArchive(
		c.api.state,
		charmURL,
		downloadedCharm,
		archive,
		size,
		bundleSHA256,
	)
}

// StoreCharmArchive stores a charm archive in environment storage.
func StoreCharmArchive(st *state.State, curl *charm.URL, ch charm.Charm, r io.Reader, size int64, sha256 string) error {
	storage := stateStorage(st)
	storagePath, err := charmArchiveStoragePath(curl)
	if err != nil {
		return errors.Annotate(err, "cannot generate charm archive name")
	}
	if err := storage.Put(storagePath, r, size); err != nil {
		return errors.Annotate(err, "cannot add charm to storage")
	}

	// Now update the charm data in state and mark it as no longer pending.
	_, err = st.UpdateUploadedCharm(ch, curl, storagePath, sha256)
	if err != nil {
		alreadyUploaded := err == state.ErrCharmRevisionAlreadyModified ||
			errors.Cause(err) == state.ErrCharmRevisionAlreadyModified ||
			state.IsCharmAlreadyUploadedError(err)
		if err := storage.Remove(storagePath); err != nil {
			if alreadyUploaded {
				logger.Errorf("cannot remove duplicated charm archive from storage: %v", err)
			} else {
				logger.Errorf("cannot remove unsuccessfully recorded charm archive from storage: %v", err)
			}
		}
		if alreadyUploaded {
			// Somebody else managed to upload and update the charm in
			// state before us. This is not an error.
			return nil
		}
	}
	return nil
}

func (c *Client) ResolveCharms(args params.ResolveCharms) (params.ResolveCharmResults, error) {
	var results params.ResolveCharmResults

	envConfig, err := c.api.state.EnvironConfig()
	if err != nil {
		return params.ResolveCharmResults{}, err
	}
	repo := config.SpecializeCharmRepo(CharmStore, envConfig)

	for _, ref := range args.References {
		result := params.ResolveCharmResult{}
		curl, err := c.resolveCharm(&ref, repo)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.URL = curl
		}
		results.URLs = append(results.URLs, result)
	}
	return results, nil
}

func (c *Client) resolveCharm(ref *charm.Reference, repo charm.Repository) (*charm.URL, error) {
	if ref.Schema != "cs" {
		return nil, fmt.Errorf("only charm store charm references are supported, with cs: schema")
	}

	// Resolve the charm location with the repository.
	return repo.Resolve(ref)
}

// charmArchiveStoragePath returns a string that is suitable as a
// storage path, using a random UUID to avoid colliding with concurrent
// uploads.
func charmArchiveStoragePath(curl *charm.URL) (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("charms/%s-%s", curl.String(), uuid), nil
}

// RetryProvisioning marks a provisioning error as transient on the machines.
func (c *Client) RetryProvisioning(p params.Entities) (params.ErrorResults, error) {
	entityStatus := make([]params.EntityStatus, len(p.Entities))
	for i, entity := range p.Entities {
		entityStatus[i] = params.EntityStatus{Tag: entity.Tag, Data: map[string]interface{}{"transient": true}}
	}
	return c.api.statusSetter.UpdateStatus(params.SetStatus{
		Entities: entityStatus,
	})
}

// APIHostPorts returns the API host/port addresses stored in state.
func (c *Client) APIHostPorts() (result params.APIHostPortsResult, err error) {
	if result.Servers, err = c.api.state.APIHostPorts(); err != nil {
		return params.APIHostPortsResult{}, err
	}
	return result, nil
}

// EnsureAvailability ensures the availability of Juju state servers.
// DEPRECATED: remove when we stop supporting 1.20 and earlier clients.
// This API is now on the HighAvailability facade.
func (c *Client) EnsureAvailability(args params.StateServersSpecs) (params.StateServersChangeResults, error) {
	results := params.StateServersChangeResults{Results: make([]params.StateServersChangeResult, len(args.Specs))}
	for i, stateServersSpec := range args.Specs {
		result, err := highavailability.EnsureAvailabilitySingle(c.api.state, stateServersSpec)
		results.Results[i].Result = result
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}
