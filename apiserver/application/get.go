// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
)

// Get returns the configuration for a service.
func (api *API) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ApplicationGetResults{}, err
	}
	app, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	settings, err := app.ConfigSettings()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	charm, _, err := app.Charm()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	configInfo := describe(settings, charm.Config())
	var constraints constraints.Value
	if app.IsPrincipal() {
		constraints, err = app.Constraints()
		if err != nil {
			return params.ApplicationGetResults{}, err
		}
	}
	return params.ApplicationGetResults{
		Application: args.ApplicationName,
		Charm:       charm.Meta().Name,
		Config:      configInfo,
		Constraints: constraints,
		Series:      app.Series(),
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
