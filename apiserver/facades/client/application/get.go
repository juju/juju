// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
)

// Get returns the configuration for a service.
func (api *API) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	return api.getApplicationSettings(args, describe)
}

// Get returns the configuration for a service.
// This used the confusing "default" boolean to mean the value was set from
// the charm defaults. Needs to be kept for backwards compatibility.
func (api *APIv4) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	return api.getApplicationSettings(args, describeV4)
}

// Get returns the configuration for a service.
func (api *API) getApplicationSettings(
	args params.ApplicationGet,
	describe func(settings charm.Settings, config *charm.Config) map[string]interface{},
) (params.ApplicationGetResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ApplicationGetResults{}, err
	}
	app, err := api.backend.Application(args.ApplicationName)
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
			"source":      "unset",
		}
		set := false
		if value := settings[name]; value != nil {
			set = true
			info["value"] = value
			info["source"] = "user"
		}
		if option.Default != nil {
			info["default"] = option.Default
			if !set {
				info["value"] = option.Default
				info["source"] = "default"
			}
		}
		results[name] = info
	}
	return results
}

func describeV4(settings charm.Settings, config *charm.Config) map[string]interface{} {
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
