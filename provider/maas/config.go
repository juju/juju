// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{}

type maasModelConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (prov MaasEnvironProvider) newConfig(cfg *config.Config) (*maasModelConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(maasModelConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

// Schema returns the configuration schema for an environment.
func (MaasEnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p MaasEnvironProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p MaasEnvironProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (prov MaasEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating MAAS specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envCfg := &maasModelConfig{
		Config: cfg,
		attrs:  validated,
	}
	return cfg.Apply(envCfg.attrs)
}
