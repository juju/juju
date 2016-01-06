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

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.api.service")

// Client allows access to the service API end point.
type Client struct {
	base.FacadeCaller
}

// NewClient creates a new client for accessing the service api.
func NewClient(caller base.APICallCloser) *Client {
	return &Client{base.NewFacadeCaller(caller, "Service")}
}

// SetMetricCredentials sets the metric credentials for the service specified.
func (c *Client) SetMetricCredentials(service string, credentials []byte) error {
	creds := []params.ServiceMetricCredential{
		{service, credentials},
	}
	p := params.ServiceMetricCredentials{creds}
	results := new(params.ErrorResults)
	err := c.FacadeCall("SetMetricCredentials", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(results.OneError())
}

// EnvironmentUUID returns the environment UUID from the client connection.
func (c *Client) EnvironmentUUID() string {
	tag, err := c.RawAPICaller().EnvironTag()
	if err != nil {
		logger.Warningf("environ tag not an environ: %v", err)
		return ""
	}
	return tag.Id()
}

// ServiceDeploy obtains the charm, either locally or from
// the charm store, and deploys it. It allows the specification of
// requested networks that must be present on the machines where the
// service is deployed. Another way to specify networks to include/exclude
// is using constraints. Placement directives, if provided, specify the
// machine on which the charm is deployed.
func (c *Client) ServiceDeploy(
	charmURL string,
	serviceName string,
	series string,
	numUnits int,
	configYAML string,
	cons constraints.Value,
	toMachineSpec string,
	placement []*instance.Placement,
	networks []string,
	storage map[string]storage.Constraints,
) error {
	args := params.ServicesDeploy{
		Services: []params.ServiceDeploy{{
			ServiceName:   serviceName,
			Series:        series,
			CharmUrl:      charmURL,
			NumUnits:      numUnits,
			ConfigYAML:    configYAML,
			Constraints:   cons,
			ToMachineSpec: toMachineSpec,
			Placement:     placement,
			Networks:      networks,
			Storage:       storage,
		}},
	}
	var results params.ErrorResults
	var err error
	if len(placement) > 0 {
		err = c.FacadeCall("ServicesDeployWithPlacement", args, &results)
		if err != nil {
			if params.IsCodeNotImplemented(err) {
				return errors.Errorf("unsupported --to parameter %q", toMachineSpec)
			}
			return err
		}
	} else {
		err = c.FacadeCall("ServicesDeploy", args, &results)
	}
	if err != nil {
		return err
	}
	return results.OneError()
}

// ServiceGetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) ServiceGetCharmURL(serviceName string) (*charm.URL, error) {
	if c.BestAPIVersion() < 2 {
		return nil, base.OldAgentError("ServiceGetCharmURL", "2.0")
	}

	result := new(params.StringResult)
	args := params.ServiceGet{ServiceName: serviceName}
	err := c.FacadeCall("ServiceGetCharmURL", args, result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return charm.ParseURL(result.Result)
}

// ServiceSetCharm sets the charm for a given service.
func (c *Client) ServiceSetCharm(serviceName string, charmUrl string, forceSeries, forceUnits bool) error {
	if c.BestAPIVersion() < 2 {
		return base.OldAgentError("ServiceSetCharm", "2.0")
	}

	args := params.ServiceSetCharm{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		ForceSeries: forceSeries,
		ForceUnits:  forceUnits,
	}
	return c.FacadeCall("ServiceSetCharm", args, nil)
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// TODO(frankban) deprecate redundant API calls that this supercedes.
func (c *Client) ServiceUpdate(args params.ServiceUpdate) error {
	if c.BestAPIVersion() < 2 {
		return base.OldAgentError("ServiceUpdate", "2.0")
	}

	return c.FacadeCall("ServiceUpdate", args, nil)
}
