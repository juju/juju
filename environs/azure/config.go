// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var azureConfigChecker = schema.StrictFieldMap(
	schema.Fields{
	// TODO: Configuration items defined in this map, as name:type.
	},
	schema.Defaults{},
)

type azureEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

// TODO: Configuration getters here.

func (prov azureEnvironProvider) newConfig(cfg *config.Config) (*azureEnvironConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(azureEnvironConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

// Validate is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating Azure specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}

	v, err := azureConfigChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	envCfg := new(azureEnvironConfig)
	envCfg.Config = cfg
	envCfg.attrs = v.(map[string]interface{})

	// TODO: Validate settings for individual config items here.

	return cfg.Apply(envCfg.attrs)
}

// BoilerplateConfig is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) BoilerplateConfig() string {
	panic("unimplemented")
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	panic("unimplemented")
}
