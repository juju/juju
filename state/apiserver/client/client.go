// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"errors"
	"fmt"
	"strings"

	"launchpad.net/juju-core/charm"
	coreerrors "launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/statecmd"
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

// ServiceSet implements the server side of Client.ServiceSet.
func (c *Client) ServiceSet(p params.ServiceSet) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return serviceSetSettingsStrings(svc, p.Options)
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *Client) ServiceSetYAML(p params.ServiceSetYAML) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return serviceSetSettingsYAML(svc, p.Config)
}

// Resolved implements the server side of Client.Resolved.
func (c *Client) Resolved(p params.Resolved) error {
	unit, err := c.api.state.Unit(p.UnitName)
	if err != nil {
		return err
	}
	return unit.Resolve(p.Retry)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(args params.ServiceExpose) error {
	return statecmd.ServiceExpose(c.api.state, args)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(args params.ServiceUnexpose) error {
	return statecmd.ServiceUnexpose(c.api.state, args)
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
	var errs []string
	for _, name := range args.UnitNames {
		unit, err := c.api.state.Unit(name)
		switch {
		case coreerrors.IsNotFoundError(err):
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
	return statecmd.ServiceDestroy(c.api.state, args)
}

// GetServiceConstraints returns the constraints for a given service.
func (c *Client) GetServiceConstraints(args params.GetServiceConstraints) (params.GetServiceConstraintsResults, error) {
	return statecmd.GetServiceConstraints(c.api.state, args)
}

// SetServiceConstraints sets the constraints for a given service.
func (c *Client) SetServiceConstraints(args params.SetServiceConstraints) error {
	return statecmd.SetServiceConstraints(c.api.state, args)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	return statecmd.AddRelation(c.api.state, args)
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(args params.DestroyRelation) error {
	return statecmd.DestroyRelation(c.api.state, args)
}

// DestroyMachines removes a given set of machines.
func (c *Client) DestroyMachines(args params.DestroyMachines) error {
	var errs []string
	for _, id := range args.MachineNames {
		machine, err := c.api.state.Machine(id)
		switch {
		case coreerrors.IsNotFoundError(err):
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
