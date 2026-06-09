// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import (
	"context"

	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
)

var (
	configFields   = schema.Fields{}
	configDefaults = schema.Defaults{}
)

type environConfig struct {
	*config.Config
	attrs map[string]any
}

func newModelConfig(config *config.Config, attrs map[string]any) *environConfig {
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

// Schema returns the configuration schema for the unmanaged provider.
// The unmanaged provider has no provider-specific config attributes.
func (p UnmanagedProvider) Schema() configschema.Fields {
	fields, err := config.Schema(nil)
	if err != nil {
		panic(err)
	}
	return fields
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p UnmanagedProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.EnableOSRefreshUpdateKey: true,
		config.EnableOSUpgradeKey:       false,
	}, nil
}
