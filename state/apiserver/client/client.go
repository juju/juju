// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"launchpad.net/juju-core/charm"
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

// ServiceSet implements the server side of Client.ServerSet.
func (c *Client) ServiceSet(p params.ServiceSet) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	ch, _, err := svc.Charm()
	if err != nil {
		return err
	}
	changes, err := ch.Config().ParseSettingsStrings(p.Options)
	if err != nil {
		return err
	}
	return svc.UpdateConfigSettings(changes)
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *Client) ServiceSetYAML(p params.ServiceSetYAML) error {
	svc, err := c.api.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	ch, _, err := svc.Charm()
	if err != nil {
		return err
	}
	changes, err := ch.Config().ParseSettingsYAML([]byte(p.Config), p.ServiceName)
	if err != nil {
		return err
	}
	return svc.UpdateConfigSettings(changes)
}

// ServiceGet returns the configuration for a service.
func (c *Client) ServiceGet(args params.ServiceGet) (params.ServiceGetResults, error) {
	return statecmd.ServiceGet(c.api.state, args)
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
		settings, err = ch.Config().ParseSettingsStrings(args.Config)
	}
	if err != nil {
		return err
	}
	_, err = conn.DeployService(juju.DeployServiceParams{
		ServiceName:      args.ServiceName,
		Charm:            ch,
		NumUnits:         args.NumUnits,
		ConfigSettings:   settings,
		Constraints:      args.Constraints,
		ForceMachineSpec: args.ForceMachineSpec,
	})
	return err
}

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(args params.ServiceSetCharm) error {
	service, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
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
	return service.SetCharm(ch, args.Force)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(args params.AddServiceUnits) (params.AddServiceUnitsResults, error) {
	units, err := statecmd.AddServiceUnits(c.api.state, args)
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
	return statecmd.DestroyServiceUnits(c.api.state, args)
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
	entity, err := c.api.state.Annotator(args.Tag)
	if err != nil {
		return params.GetAnnotationsResults{}, err
	}
	ann, err := entity.Annotations()
	if err != nil {
		return params.GetAnnotationsResults{}, err
	}
	return params.GetAnnotationsResults{Annotations: ann}, nil
}

// SetAnnotations stores annotations about a given entity.
func (c *Client) SetAnnotations(args params.SetAnnotations) error {
	entity, err := c.api.state.Annotator(args.Tag)
	if err != nil {
		return err
	}
	return entity.SetAnnotations(args.Pairs)
}
