// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
)

// Get returns the configuration for a service.
func (api *API) Get(args params.ServiceGet) (params.ServiceGetResults, error) {
	service, err := api.state.Service(args.ServiceName)
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
