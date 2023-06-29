// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
)

// CloudService represents a service that can be used for finding Juju clouds by
// name.
type CloudService interface {
	// Get returns the named cloud
	Get(context.Context, string) (*cloud.Cloud, error)
}

// CloudDefaultsState represents the state entity responsible for storing and
// fetching ModelDefaults related to
type CloudDefaultsState interface {
	// CloudAllRegionDefaults retrieves the default values for each region of
	// the specified cloud.
	CloudAllRegionDefaults(context.Context, string) (map[string]map[string]string, error)

	// CloudDefaults retrieves the default values set for the specified cloud.
	CloudDefaults(context.Context, string) (map[string]string, error)

	// UpdateCloudDefaults is responsible for updating the default values for
	// the specified cloud.
	UpdateCloudDefaults(context.Context, string, map[string]string, []string) error

	// UpdateCloudRegionDefaults is responsible for updating the default values
	// for the specified cloud region.
	UpdateCloudRegionDefaults(context.Context, string, string, map[string]string, []string) error
}

// StaticConfigProvider is responsible for providing hooks to the default hard
// coded configuration sources within Juju.
type StaticConfigProvider interface {
	// ConfigDefaults is responsible providing the hardcoded default config
	// values located within Juju
	ConfigDefaults() map[string]any

	// CloudConfig is responsible for return the config for the specified cloud.
	// If no config is found for the specified cloud then an  error that
	// satisfies NotFound should be returned.
	CloudConfig(string) (config.ConfigSchemaSource, error)
}
