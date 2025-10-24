// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import (
	"context"

	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

var (
	configFields   = schema.Fields{}
	configDefaults = schema.Defaults{}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newModelConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	return &environConfig{Config: config, attrs: attrs}
}

// ConfigDefaults returns the default values for this providers specific config
// attributes.
func (p UnmanagedProvider) ConfigDefaults() schema.Defaults {
	return schema.Defaults{}
}

// ConfigSchema returns the extra config attributes specific to this provider.
func (p UnmanagedProvider) ConfigSchema() schema.Fields {
	return schema.Fields{}
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p UnmanagedProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.EnableOSRefreshUpdateKey: true,
		config.EnableOSUpgradeKey:       false,
	}, nil
}
