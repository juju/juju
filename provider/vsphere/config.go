// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

// The vmware-specific config keys.
const (
	cfgExternalNetwork = "external-network"
)

// configFields is the spec for each vmware config value's type.
var (
	configFields = schema.Fields{
		cfgExternalNetwork: schema.String(),
	}

	requiredFields = []string{}

	configDefaults = schema.Defaults{
		cfgExternalNetwork: "",
	}

	configImmutableFields = []string{}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

// newConfig builds a new environConfig from the provided Config and
// returns it.
func newConfig(cfg *config.Config) *environConfig {
	return &environConfig{
		Config: cfg,
		attrs:  cfg.UnknownAttrs(),
	}
}

// newValidConfig builds a new environConfig from the provided Config
// and returns it. The resulting config values are validated.
func newValidConfig(cfg *config.Config, defaults map[string]interface{}) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Apply the defaults and coerce/validate the custom config attrs.
	validated, err := cfg.ValidateUnknownAttrs(configFields, defaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validCfg, err := cfg.Apply(validated)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the config.
	ecfg := newConfig(validCfg)

	// Do final validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
}

func (c *environConfig) externalNetwork() string {
	return c.attrs[cfgExternalNetwork].(string)
}

// validate checks vmware-specific config values.
func (c environConfig) validate() error {
	// All fields must be populated, even with just the default.
	for _, field := range requiredFields {
		if c.attrs[field].(string) == "" {
			return errors.Errorf("%s: must not be empty", field)
		}
	}
	return nil
}

// update applies changes from the provided config to the env config.
// Changes to any immutable attributes result in an error.
func (c *environConfig) update(cfg *config.Config) error {
	// Validate the updates. newValidConfig does not modify the "known"
	// config attributes so it is safe to call Validate here first.
	if err := config.Validate(cfg, c.Config); err != nil {
		return errors.Trace(err)
	}

	updates, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return errors.Trace(err)
	}

	// Check that no immutable fields have changed.
	attrs := updates.UnknownAttrs()
	for _, field := range configImmutableFields {
		if attrs[field] != c.attrs[field] {
			return errors.Errorf("%s: cannot change from %v to %v", field, c.attrs[field], attrs[field])
		}
	}

	// Apply the updates.
	c.Config = cfg
	c.attrs = cfg.UnknownAttrs()
	return nil
}
