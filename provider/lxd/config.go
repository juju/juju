// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	"project": {
		Description: "The LXD project name to use for Juju's resources.",
		Type:        environschema.Tstring,
	},
}

var configDefaults = schema.Defaults{
	"project": "default",
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

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
// and returns it. This includes applying the provided defaults
// values, if any. The resulting config values are validated.
func newValidConfig(cfg *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Build the config.
	ecfg := newConfig(cfg)

	// Do final (more complex, provider-specific) validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
}

// validate validates LXD-specific configuration.
func (c *environConfig) validate() error {
	_, err := c.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return errors.Trace(err)
	}
	// There are currently no known extra fields for LXD
	return nil
}

func (c *environConfig) project() string {
	project := c.attrs["project"]
	if project == nil {
		return ""
	}
	return project.(string)
}
