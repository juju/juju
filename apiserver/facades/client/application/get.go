// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/schema"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/constraints"
)

// Get returns the charm configuration for an application.
func (api *APIBase) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	return api.getConfig(args, describe)
}

// Get returns the charm configuration for an application.
// It zeros out any application config as that was not supported in v5.
func (api *APIv5) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	results, err := api.getConfig(args, describe)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	results.ApplicationConfig = nil
	return results, nil
}

// Get returns the charm configuration for an application.
// This used the confusing "default" boolean to mean the value was set from
// the charm defaults. Needs to be kept for backwards compatibility.
func (api *APIv4) Get(args params.ApplicationGet) (params.ApplicationGetResults, error) {
	results, err := api.getConfig(args, describeV4)
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	results.ApplicationConfig = nil
	return results, nil
}

// Get returns the charm configuration for an application.
func (api *APIBase) getConfig(
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
	settings, err := app.CharmConfig()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	charm, _, err := app.Charm()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	configInfo := describe(settings, charm.Config())
	appConfig, err := app.ApplicationConfig()
	if err != nil {
		return params.ApplicationGetResults{}, err
	}

	providerSchema, providerDefaults, err := applicationConfigSchema(api.backend.ModelType())
	if err != nil {
		return params.ApplicationGetResults{}, err
	}
	appConfigInfo := describeAppConfig(appConfig, providerSchema, caas.ConfigDefaults(providerDefaults))
	var constraints constraints.Value
	if app.IsPrincipal() {
		constraints, err = app.Constraints()
		if err != nil {
			return params.ApplicationGetResults{}, err
		}
	}
	return params.ApplicationGetResults{
		Application:       args.ApplicationName,
		Charm:             charm.Meta().Name,
		CharmConfig:       configInfo,
		ApplicationConfig: appConfigInfo,
		Constraints:       constraints,
		Series:            app.Series(),
	}, nil
}

func describeAppConfig(
	appConfig application.ConfigAttributes,
	schema environschema.Fields,
	defaults schema.Defaults,
) map[string]interface{} {
	results := make(map[string]interface{})
	for name, field := range schema {
		defaultValue := defaults[name]
		info := map[string]interface{}{
			"description": field.Description,
			"type":        field.Type,
			"source":      "unset",
		}
		set := false
		if value := appConfig[name]; value != nil && defaultValue != value {
			set = true
			info["value"] = value
			info["source"] = "user"
		}
		if defaultValue != nil {
			info["default"] = defaultValue
			if !set {
				info["value"] = defaultValue
				info["source"] = "default"
			}
		}
		results[name] = info
	}
	return results
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
		if value := settings[name]; value != nil && option.Default != value {
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
		if value := settings[name]; value != nil && option.Default != value {
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
