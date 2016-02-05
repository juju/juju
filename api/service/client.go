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
	creds := []params.ServiceMetricCredential{
		{service, credentials},
	}
	p := params.ServiceMetricCredentials{creds}
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

// Deploy obtains the charm, either locally or from
// the charm store, and deploys it. It allows the specification of
// requested networks that must be present on the machines where the
// service is deployed. Another way to specify networks to include/exclude
// is using constraints. Placement directives, if provided, specify the
// machine on which the charm is deployed.
func (c *Client) Deploy(
	charmURL string,
	serviceName string,
	series string,
	numUnits int,
	configYAML string,
	cons constraints.Value,
	placement []*instance.Placement,
	networks []string,
	storage map[string]storage.Constraints,
) error {
	args := params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			ServiceName: serviceName,
			Series:      series,
			CharmUrl:    charmURL,
			NumUnits:    numUnits,
			ConfigYAML:  configYAML,
			Constraints: cons,
			Placement:   placement,
			Networks:    networks,
			Storage:     storage,
		}},
	}
	var results params.ErrorResults
	var err error
	err = c.facade.FacadeCall("Deploy", args, &results)
	if err != nil {
		return err
	}
	return results.OneError()
}

// GetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) GetCharmURL(serviceName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ServiceGet{ServiceName: serviceName}
	err := c.facade.FacadeCall("GetCharmURL", args, result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return charm.ParseURL(result.Result)
}

// SetCharm sets the charm for a given service.
func (c *Client) SetCharm(serviceName string, charmUrl string, forceSeries, forceUnits bool) error {
	args := params.ServiceSetCharm{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		ForceSeries: forceSeries,
		ForceUnits:  forceUnits,
	}
	return c.facade.FacadeCall("SetCharm", args, nil)
}

// Update updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
func (c *Client) Update(args params.ServiceUpdate) error {
	return c.facade.FacadeCall("Update", args, nil)
}

// AddUnits adds a given number of units to a service using the specified
// placement directives to assign units to machines.
func (c *Client) AddUnits(service string, numUnits int, placement []*instance.Placement) ([]string, error) {
	args := params.AddServiceUnits{
		ServiceName: service,
		NumUnits:    numUnits,
		Placement:   placement,
	}
	results := new(params.AddServiceUnitsResults)
	err := c.facade.FacadeCall("AddUnits", args, results)
	return results.Units, err
}

// DestroyUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyUnits(unitNames ...string) error {
	params := params.DestroyServiceUnits{unitNames}
	return c.facade.FacadeCall("DestroyUnits", params, nil)
}

// Destroy destroys a given service.
func (c *Client) Destroy(service string) error {
	params := params.ServiceDestroy{
		ServiceName: service,
	}
	return c.facade.FacadeCall("Destroy", params, nil)
}

// GetConstraints returns the constraints for the given service.
func (c *Client) GetConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetConstraints", params.GetServiceConstraints{service}, results)
	return results.Constraints, err
}

// SetConstraints specifies the constraints for the given service.
func (c *Client) SetConstraints(service string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ServiceName: service,
		Constraints: constraints,
	}
	return c.facade.FacadeCall("SetConstraints", params, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) Expose(service string) error {
	params := params.ServiceExpose{ServiceName: service}
	return c.facade.FacadeCall("Expose", params, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(service string) error {
	params := params.ServiceUnexpose{ServiceName: service}
	return c.facade.FacadeCall("Unexpose", params, nil)
}

// Get returns the configuration for the named service.
func (c *Client) Get(service string) (*params.ServiceGetResults, error) {
	var results params.ServiceGetResults
	params := params.ServiceGet{ServiceName: service}
	err := c.facade.FacadeCall("Get", params, &results)
	return &results, err
}

// Set sets configuration options on a service.
func (c *Client) Set(service string, options map[string]string) error {
	p := params.ServiceSet{
		ServiceName: service,
		Options:     options,
	}
	return c.facade.FacadeCall("Set", p, nil)
}

// Unset resets configuration options on a service.
func (c *Client) Unset(service string, options []string) error {
	p := params.ServiceUnset{
		ServiceName: service,
		Options:     options,
	}
	return c.facade.FacadeCall("Unset", p, nil)
}

// CharmRelations returns the service's charms relation names.
func (c *Client) CharmRelations(service string) ([]string, error) {
	var results params.ServiceCharmRelationsResults
	params := params.ServiceCharmRelations{ServiceName: service}
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
