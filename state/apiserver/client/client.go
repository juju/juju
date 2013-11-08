// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

type API struct {
	state     *state.State
	auth      common.Authorizer
	resources *common.Resources
	client    *Client
}

// Client serves client-specific API methods.
type Client struct {
	api *API
}

// NewAPI creates a new instance of the Client API.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) *API {
	r := &API{
		state:     st,
		auth:      authorizer,
		resources: resources,
	}
	r.client = &Client{
		api: r,
	}
	return r
}

// Client returns an object that provides access
// to methods accessible to non-agent clients.
func (r *API) Client(id string) (*Client, error) {
	if !r.auth.AuthClient() {
		return nil, common.ErrPerm
	}
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.client, nil
}

func (c *Client) Status() (api.Status, error) {
	ms, err := c.api.state.AllMachines()
	if err != nil {
		return api.Status{}, err
	}
	status := api.Status{
		Machines: make(map[string]api.MachineInfo),
	}
	for _, m := range ms {
		instId, err := m.InstanceId()
		if err != nil && !state.IsNotProvisionedError(err) {
			return api.Status{}, err
		}
		status.Machines[m.Id()] = api.MachineInfo{
			InstanceId: string(instId),
		}
	}
	return status, nil
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
	case names.IsMachine(p.Target):
		machine, err := c.api.state.Machine(p.Target)
		if err != nil {
			return results, err
		}
		addr := instance.SelectPublicAddress(machine.Addresses())
		if addr == "" {
			return results, fmt.Errorf("machine %q has no public address", machine)
		}
		return params.PublicAddressResults{PublicAddress: addr}, nil

	case names.IsUnit(p.Target):
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

// ServiceDeploy fetches the charm from the charm store and deploys it. Local
// charms are not supported.
func (c *Client) ServiceDeploy(args params.ServiceDeploy) error {
	curl, err := charm.ParseURL(args.CharmUrl)
	if err != nil {
		return err
	}
	if curl.Schema != "cs" {
		return fmt.Errorf(`charm url has unsupported schema %q`, curl.Schema)
	}
	if curl.Revision < 0 {
		return fmt.Errorf("charm url must include revision")
	}
	conn, err := juju.NewConnFromState(c.api.state)
	if err != nil {
		return err
	}
	ch, err := conn.PutCharm(curl, CharmStore, false)
	if err != nil {
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
	_, err = conn.DeployService(juju.DeployServiceParams{
		ServiceName:    args.ServiceName,
		Charm:          ch,
		NumUnits:       args.NumUnits,
		ConfigSettings: settings,
		Constraints:    args.Constraints,
		ToMachineSpec:  args.ToMachineSpec,
	})
	return err
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
		if err = serviceSetCharm(c.api.state, service, args.CharmUrl, args.ForceCharmUrl); err != nil {
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
func serviceSetCharm(state *state.State, service *state.Service, url string, force bool) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return err
	}
	if curl.Schema != "cs" {
		return fmt.Errorf(`charm url has unsupported schema %q`, curl.Schema)
	}
	if curl.Revision < 0 {
		return fmt.Errorf("charm url must include revision")
	}
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	ch, err := conn.PutCharm(curl, CharmStore, false)
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
	return serviceSetCharm(c.api.state, service, args.CharmUrl, args.Force)
}

// addServiceUnits adds a given number of units to a service.
// TODO(jam): 2013-08-26 https://pad.lv/1216830
// The functionality on conn.AddUnits should get pulled up into
// state/apiserver/client, but currently we still have conn.DeployService that
// depends on it. When that changes, clean up this function.
func addServiceUnits(state *state.State, args params.AddServiceUnits) ([]*state.Unit, error) {
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return nil, err
	}
	service, err := state.Service(args.ServiceName)
	if err != nil {
		return nil, err
	}
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
		return nil, errors.New("cannot use NumUnits with ToMachineSpec")
	}
	return conn.AddUnits(service, args.NumUnits, args.ToMachineSpec)
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
	return c.api.state.DestroyUnits(args.UnitNames...)
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
	inEps, err := c.api.state.InferEndpoints(args.Endpoints)
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
	eps, err := c.api.state.InferEndpoints(args.Endpoints)
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
	results := params.AddMachinesResults{
		Machines: make([]params.AddMachinesResult, len(args.MachineParams)),
	}

	var defaultSeries string
	conf, err := c.api.state.EnvironConfig()
	if err != nil {
		return results, err
	}
	defaultSeries = conf.DefaultSeries()

	for i, machineParams := range args.MachineParams {
		series := machineParams.Series
		if series == "" {
			series = defaultSeries
		}
		stateMachineParams := state.AddMachineParams{
			Series:        series,
			Constraints:   machineParams.Constraints,
			ParentId:      machineParams.ParentId,
			ContainerType: machineParams.ContainerType,
		}
		// Convert params.MachineJob to state.MachineJob
		stateMachineParams.Jobs = make([]state.MachineJob, len(machineParams.Jobs))
		var jobError error
		for j, job := range machineParams.Jobs {
			if stateMachineParams.Jobs[j], jobError = state.MachineJobFromParams(job); jobError != nil {
				break
			}
		}
		if jobError != nil {
			results.Machines[i].Error = common.ServerError(jobError)
			continue
		}
		machine, err := c.api.state.AddMachineWithConstraints(&stateMachineParams)
		results.Machines[i].Error = common.ServerError(err)
		if err == nil {
			results.Machines[i].Machine = machine.String()
		}
	}
	return results, nil
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	return c.api.state.DestroyMachines(args.MachineNames...)
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
		DefaultSeries: conf.DefaultSeries(),
		ProviderType:  conf.Type(),
		Name:          conf.Name(),
		UUID:          env.UUID(),
	}
	return info, nil
}

// GetAnnotations returns annotations about a given entity.
func (c *Client) GetAnnotations(args params.GetAnnotations) (params.GetAnnotationsResults, error) {
	nothing := params.GetAnnotationsResults{}
	entity, err := c.findEntity(args.Tag)
	if err != nil {
		return nothing, err
	}
	ann, err := entity.Annotations()
	if err != nil {
		return nothing, err
	}
	return params.GetAnnotationsResults{Annotations: ann}, nil
}

func (c *Client) findEntity(tag string) (state.Annotator, error) {
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
	entity, err := c.findEntity(args.Tag)
	if err != nil {
		return err
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
	// TODO(dimitern,thumper): 2013-11-06 bug #1167616
	// SetEnvironConfig should take both new and old configs.

	// Get the existing environment config from the state.
	oldConfig, err := c.api.state.EnvironConfig()
	if err != nil {
		return err
	}
	// Make sure we don't allow changing agent-version.
	if v, found := args.Config["agent-version"]; found {
		oldVersion, _ := oldConfig.AgentVersion()
		if v != oldVersion.String() {
			return fmt.Errorf("agent-version cannot be changed")
		}
	}
	// Apply the attributes specified for the command to the state config.
	newConfig, err := oldConfig.Apply(args.Config)
	if err != nil {
		return err
	}
	env, err := environs.New(oldConfig)
	if err != nil {
		return err
	}
	// Now validate this new config against the existing config via the provider.
	provider := env.Provider()
	newProviderConfig, err := provider.Validate(newConfig, oldConfig)
	if err != nil {
		return err
	}
	// Now try to apply the new validated config.
	return c.api.state.SetEnvironConfig(newProviderConfig)
}
