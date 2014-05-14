// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state/api/params"
)

// ServiceGet returns the configuration for a service.
func (c *Client) ServiceGet(args params.ServiceGet) (params.ServiceGetResults, error) {
	service, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	settings, err := service.ConfigSettings()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	charm, _, err := service.Charm()
	if err != nil {
		return params.ServiceGetResults{}, err
	}
	configInfo := describe(settings, charm.Config())
	var constraints constraints.Value
	if service.IsPrincipal() {
		constraints, err = service.Constraints()
		if err != nil {
			return params.ServiceGetResults{}, err
		}
	}
	return params.ServiceGetResults{
		Service:     args.ServiceName,
		Charm:       charm.Meta().Name,
		Config:      configInfo,
		Constraints: constraints,
	}, nil
}

func describe(settings charm.Settings, config *charm.Config) map[string]interface{} {
	results := make(map[string]interface{})
	for name, option := range config.Options {
		info := map[string]interface{}{
			"description": option.Description,
			"type":        option.Type,
		}
		if value := settings[name]; value != nil {
			info["value"] = value
		} else {
			if option.Default != nil {
				info["value"] = option.Default
			}
			info["default"] = true
		}
		results[name] = info
	}
	return results
}

// ServiceGetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) ServiceGetCharmURL(args params.ServiceGet) (params.StringResult, error) {
	service, err := c.api.state.Service(args.ServiceName)
	if err != nil {
		return params.StringResult{}, err
	}
	charmURL, _ := service.CharmURL()
	return params.StringResult{Result: charmURL.String()}, nil
}
