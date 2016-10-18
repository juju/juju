// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"

	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

var (
	configFields          = schema.Fields{}
	configDefaults        = schema.Defaults{}
	requiredFields        = []string{}
	configImmutableFields = []string{}
)

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}

	newAttrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := &environConfig{cfg, newAttrs}
	// If an old config was supplied, check any immutable fields have not changed.
	if old != nil {
		oldEnvConfig, err := validateConfig(old, nil)
		if err != nil {
			return nil, err
		}
		for _, field := range configImmutableFields {
			if oldEnvConfig.attrs[field] != envConfig.attrs[field] {
				return nil, fmt.Errorf(
					"%s: cannot change from %v to %v",
					field, oldEnvConfig.attrs[field], envConfig.attrs[field],
				)
			}
		}
	}

	// Check for missing fields.
	for _, field := range requiredFields {
		if nilOrEmptyString(envConfig.attrs[field]) {
			return nil, fmt.Errorf("%s: must not be empty", field)
		}
	}
	return envConfig, nil
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (ecfg *environConfig) GetAttrs() map[string]interface{} {
	return ecfg.attrs
}

func nilOrEmptyString(i interface{}) bool {
	return i == nil || i == ""
}
