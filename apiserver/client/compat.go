// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
)

// TODO(wallyworld) - deprecated, remove when GUI updated.
// compat is used to provide shims used to allow deprecated APIs off the client
// facade to call the new functionality.
type compat struct {
	client *Client
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *compat) ServiceSetYAML(p params.ServiceSetYAML) error {
	serviceApi, err := service.NewAPI(c.client.api.state(), c.client.api.resources, c.client.api.auth)
	if err != nil {
		return err
	}
	return serviceApi.ServiceUpdate(params.ServiceUpdate{
		ServiceName:  p.ServiceName,
		SettingsYAML: p.Config,
	})
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// All parameters in params.ServiceUpdate except the service name are optional.
func (c *compat) ServiceUpdate(args params.ServiceUpdate) error {
	serviceApi, err := service.NewAPI(c.client.api.state(), c.client.api.resources, c.client.api.auth)
	if err != nil {
		return err
	}
	return serviceApi.ServiceUpdate(args)
}

// ServiceSetCharm sets the charm for a given service.
func (c *compat) ServiceSetCharm(args params.ServiceSetCharm) error {
	// Older Juju's required a charm to be added first; the GUI assumes this behaviour.
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
	_, err = c.client.api.state().Charm(curl)
	if errors.IsNotFound(err) {
		if err := c.client.AddCharm(params.CharmURL{URL: curl.String()}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	serviceApi, err := service.NewAPI(c.client.api.state(), c.client.api.resources, c.client.api.auth)
	if err != nil {
		return err
	}
	return serviceApi.ServiceSetCharm(args)
}

// ServiceGetCharmURL gets the charm URL for a given service.
func (c *compat) ServiceGetCharmURL(args params.ServiceGet) (params.StringResult, error) {
	serviceApi, err := service.NewAPI(c.client.api.state(), c.client.api.resources, c.client.api.auth)
	if err != nil {
		return params.StringResult{}, err
	}
	return serviceApi.ServiceGetCharmURL(args)
}
