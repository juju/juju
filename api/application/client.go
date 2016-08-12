// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application provides access to the application api facade.
// This facade contains api calls that are specific to applications.
// As a rule of thumb, if the argument for an api requires an application name
// and affects only that application then the call belongs here.
package application

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

var logger = loggo.GetLogger("juju.api.application")

// Client allows access to the service API end point.
type Client struct {
	base.ClientFacade
	st     api.Connection
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the application api.
func NewClient(st api.Connection) *Client {
	frontend, backend := base.NewClientFacade(st, "Application")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// SetMetricCredentials sets the metric credentials for the application specified.
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
	tag, ok := c.st.ModelTag()
	if !ok {
		logger.Warningf("controller-only API connection has no model tag")
	}
	return tag.Id()
}

// DeployArgs holds the arguments to be sent to Client.ServiceDeploy.
type DeployArgs struct {
	// CharmID identifies the charm to deploy.
	CharmID charmstore.CharmID
	// ApplicationName is the name to give the application.
	ApplicationName string
	// Series to be used for the machine.
	Series string
	// NumUnits is the number of units to deploy.
	NumUnits int
	// ConfigYAML is a string that overrides the default config.yml.
	ConfigYAML string
	// Cons contains constraints on where units of this application may be
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
	// Collection of resource names for the application, with the value being the
	// unique ID of a pre-uploaded resources in storage.
	Resources map[string]string
}

// Deploy obtains the charm, either locally or from the charm store, and deploys
// it. Placement directives, if provided, specify the machine on which the charm
// is deployed.
func (c *Client) Deploy(args DeployArgs) error {
	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName:  args.ApplicationName,
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
	// ApplicationName is the name of the application to set the charm on.
	ApplicationName string
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
		ApplicationName: cfg.ApplicationName,
		CharmUrl:        cfg.CharmID.URL.String(),
		Channel:         string(cfg.CharmID.Channel),
		ForceSeries:     cfg.ForceSeries,
		ForceUnits:      cfg.ForceUnits,
		ResourceIDs:     cfg.ResourceIDs,
	}
	return c.facade.FacadeCall("SetCharm", args, nil)
}

// Update updates the application attributes, including charm URL,
// minimum number of units, settings and constraints.
func (c *Client) Update(args params.ApplicationUpdate) error {
	return c.facade.FacadeCall("Update", args, nil)
}

// AddUnits adds a given number of units to an application using the specified
// placement directives to assign units to machines.
func (c *Client) AddUnits(application string, numUnits int, placement []*instance.Placement) ([]string, error) {
	args := params.AddApplicationUnits{
		ApplicationName: application,
		NumUnits:        numUnits,
		Placement:       placement,
	}
	results := new(params.AddApplicationUnitsResults)
	err := c.facade.FacadeCall("AddUnits", args, results)
	return results.Units, err
}

// DestroyUnits decreases the number of units dedicated to an application.
func (c *Client) DestroyUnits(unitNames ...string) error {
	params := params.DestroyApplicationUnits{unitNames}
	return c.facade.FacadeCall("DestroyUnits", params, nil)
}

// Destroy destroys a given application.
func (c *Client) Destroy(application string) error {
	params := params.ApplicationDestroy{
		ApplicationName: application,
	}
	return c.facade.FacadeCall("Destroy", params, nil)
}

// GetConstraints returns the constraints for the given application.
func (c *Client) GetConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetConstraints", params.GetApplicationConstraints{service}, results)
	return results.Constraints, err
}

// SetConstraints specifies the constraints for the given application.
func (c *Client) SetConstraints(application string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ApplicationName: application,
		Constraints:     constraints,
	}
	return c.facade.FacadeCall("SetConstraints", params, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) Expose(application string) error {
	params := params.ApplicationExpose{ApplicationName: application}
	return c.facade.FacadeCall("Expose", params, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(application string) error {
	params := params.ApplicationUnexpose{ApplicationName: application}
	return c.facade.FacadeCall("Unexpose", params, nil)
}

// Get returns the configuration for the named application.
func (c *Client) Get(application string) (*params.ApplicationGetResults, error) {
	var results params.ApplicationGetResults
	params := params.ApplicationGet{ApplicationName: application}
	err := c.facade.FacadeCall("Get", params, &results)
	return &results, err
}

// Set sets configuration options on an application.
func (c *Client) Set(application string, options map[string]string) error {
	p := params.ApplicationSet{
		ApplicationName: application,
		Options:         options,
	}
	return c.facade.FacadeCall("Set", p, nil)
}

// Unset resets configuration options on an application.
func (c *Client) Unset(application string, options []string) error {
	p := params.ApplicationUnset{
		ApplicationName: application,
		Options:         options,
	}
	return c.facade.FacadeCall("Unset", p, nil)
}

// CharmRelations returns the application's charms relation names.
func (c *Client) CharmRelations(application string) ([]string, error) {
	var results params.ApplicationCharmRelationsResults
	params := params.ApplicationCharmRelations{ApplicationName: application}
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
