// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service provides access to the service api facade.
// This facade contains api calls that are specific to services.
// As a rule of thumb, if the argument for an api requries a service name
// and affects only that service then the call belongs here.
package service

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.api.service")

// Client allows access to the service API end point.
type Client struct {
	base.ClientFacade
	st     api.Connection
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the service api.
func NewClient(st api.Connection) *Client {
	frontend, backend := base.NewClientFacade(st, "Service")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// SetMetricCredentials sets the metric credentials for the service specified.
func (c *Client) SetMetricCredentials(service string, credentials []byte) error {
	creds := []params.ApplicationMetricCredential{
		{service, credentials},
	}
	p := params.ApplicationMetricCredentials{creds}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("SetMetricCredentials", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(results.OneError())
}

// ModelUUID returns the model UUID from the client connection.
func (c *Client) ModelUUID() string {
	tag, err := c.st.ModelTag()
	if err != nil {
		logger.Warningf("model tag not an model: %v", err)
		return ""
	}
	return tag.Id()
}

// DeployArgs holds the arguments to be sent to Client.ServiceDeploy.
type DeployArgs struct {
	// CharmID identifies the charm to deploy.
	CharmID charmstore.CharmID
	// ServiceName is the name to give the service.
	ServiceName string
	// Series to be used for the machine.
	Series string
	// NumUnits is the number of units to deploy.
	NumUnits int
	// ConfigYAML is a string that overrides the default config.yml.
	ConfigYAML string
	// Cons contains constraints on where units of this service may be
	// placed.
	Cons constraints.Value
	// Placement directives on where the machines for the unit must be
	// created.
	Placement []*instance.Placement
	// Storage contains Constraints specifying how storage should be
	// handled.
	Storage map[string]storage.Constraints
	// EndpointBindings
	EndpointBindings map[string]string
	// Collection of resource names for the service, with the value being the
	// unique ID of a pre-uploaded resources in storage.
	Resources map[string]string
}

// Deploy obtains the charm, either locally or from the charm store, and deploys
// it. Placement directives, if provided, specify the machine on which the charm
// is deployed.
func (c *Client) Deploy(args DeployArgs) error {
	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName:  args.ServiceName,
			Series:           args.Series,
			CharmUrl:         args.CharmID.URL.String(),
			Channel:          string(args.CharmID.Channel),
			NumUnits:         args.NumUnits,
			ConfigYAML:       args.ConfigYAML,
			Constraints:      args.Cons,
			Placement:        args.Placement,
			Storage:          args.Storage,
			EndpointBindings: args.EndpointBindings,
			Resources:        args.Resources,
		}},
	}
	var results params.ErrorResults
	var err error
	err = c.facade.FacadeCall("Deploy", deployArgs, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}

// GetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) GetCharmURL(serviceName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ApplicationGet{ApplicationName: serviceName}
	err := c.facade.FacadeCall("GetCharmURL", args, result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return charm.ParseURL(result.Result)
}

// SetCharmConfig holds the configuration for setting a new revision of a charm
// on a service.
type SetCharmConfig struct {
	// ServiceName is the name of the service to set the charm on.
	ServiceName string
	// CharmID identifies the charm.
	CharmID charmstore.CharmID
	// ForceSeries forces the use of the charm even if it doesn't match the
	// series of the unit.
	ForceSeries bool
	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool
	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string
}

// SetCharm sets the charm for a given service.
func (c *Client) SetCharm(cfg SetCharmConfig) error {
	args := params.ApplicationSetCharm{
		ApplicationName: cfg.ServiceName,
		CharmUrl:        cfg.CharmID.URL.String(),
		Channel:         string(cfg.CharmID.Channel),
		ForceSeries:     cfg.ForceSeries,
		ForceUnits:      cfg.ForceUnits,
		ResourceIDs:     cfg.ResourceIDs,
	}
	return c.facade.FacadeCall("SetCharm", args, nil)
}

// Update updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
func (c *Client) Update(args params.ApplicationUpdate) error {
	return c.facade.FacadeCall("Update", args, nil)
}

// AddUnits adds a given number of units to a service using the specified
// placement directives to assign units to machines.
func (c *Client) AddUnits(service string, numUnits int, placement []*instance.Placement) ([]string, error) {
	args := params.AddApplicationUnits{
		ApplicationName: service,
		NumUnits:        numUnits,
		Placement:       placement,
	}
	results := new(params.AddApplicationUnitsResults)
	err := c.facade.FacadeCall("AddUnits", args, results)
	return results.Units, err
}

// DestroyUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyUnits(unitNames ...string) error {
	params := params.DestroyApplicationUnits{unitNames}
	return c.facade.FacadeCall("DestroyUnits", params, nil)
}

// Destroy destroys a given service.
func (c *Client) Destroy(service string) error {
	params := params.ApplicationDestroy{
		ApplicationName: service,
	}
	return c.facade.FacadeCall("Destroy", params, nil)
}

// GetConstraints returns the constraints for the given service.
func (c *Client) GetConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetConstraints", params.GetApplicationConstraints{service}, results)
	return results.Constraints, err
}

// SetConstraints specifies the constraints for the given service.
func (c *Client) SetConstraints(service string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ApplicationName: service,
		Constraints:     constraints,
	}
	return c.facade.FacadeCall("SetConstraints", params, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) Expose(service string) error {
	params := params.ApplicationExpose{ApplicationName: service}
	return c.facade.FacadeCall("Expose", params, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(service string) error {
	params := params.ApplicationUnexpose{ApplicationName: service}
	return c.facade.FacadeCall("Unexpose", params, nil)
}

// Get returns the configuration for the named service.
func (c *Client) Get(service string) (*params.ApplicationGetResults, error) {
	var results params.ApplicationGetResults
	params := params.ApplicationGet{ApplicationName: service}
	err := c.facade.FacadeCall("Get", params, &results)
	return &results, err
}

// Set sets configuration options on a service.
func (c *Client) Set(service string, options map[string]string) error {
	p := params.ApplicationSet{
		ApplicationName: service,
		Options:         options,
	}
	return c.facade.FacadeCall("Set", p, nil)
}

// Unset resets configuration options on a service.
func (c *Client) Unset(service string, options []string) error {
	p := params.ApplicationUnset{
		ApplicationName: service,
		Options:         options,
	}
	return c.facade.FacadeCall("Unset", p, nil)
}

// CharmRelations returns the service's charms relation names.
func (c *Client) CharmRelations(service string) ([]string, error) {
	var results params.ApplicationCharmRelationsResults
	params := params.ApplicationCharmRelations{ApplicationName: service}
	err := c.facade.FacadeCall("CharmRelations", params, &results)
	return results.CharmRelations, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints}
	err := c.facade.FacadeCall("AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.facade.FacadeCall("DestroyRelation", params, nil)
}
