// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/schema"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/internal/configschema"
)

// TODO(dqlite) - we don't want to reference environs/config here but need a place for config key names.
const (
	// StorageDefaultBlockSourceKey is the key for the default block storage source.
	StorageDefaultBlockSourceKey = "storage-default-block-source"

	// StorageDefaultFilesystemSourceKey is the key for the default filesystem storage source.
	StorageDefaultFilesystemSourceKey = "storage-default-filesystem-source"
)

// ConfigSchema returns the config schema and defaults for an application.
func ApplicationConfigSchema() (configschema.Fields, schema.Defaults) {
	return trustFields, trustDefaults
}

const defaultTrustLevel = false

var trustFields = configschema.Fields{
	application.TrustConfigOptionName: {
		Description: "Does this application have access to trusted credentials",
		Type:        configschema.Tbool,
		Group:       configschema.JujuGroup,
	},
}

var trustDefaults = schema.Defaults{
	application.TrustConfigOptionName: defaultTrustLevel,
}
