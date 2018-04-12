// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"
)

const (
	// JujuManagedUnits specifies whether Juju or the CAAS substrate manages unit lifecycle.
	JujuManagedUnits = "juju-managed-units"

	// JujuDefaultJujuManagedUnits is the default value for juju-managed-units.
	JujuDefaultJujuManagedUnits = false

	// JujuExternalHostNameKey specifies the hostname of a CAAS application.
	JujuExternalHostNameKey = "juju-external-hostname"

	// JujuApplicationPath specifies the relative http path used to access a CAAS application.
	JujuApplicationPath = "juju-application-path"

	// JujuDefaultApplicationPath is the default value for juju-application-path.
	JujuDefaultApplicationPath = "/"
)

var configFields = environschema.Fields{
	JujuManagedUnits: {
		Description: "whether Juju manages unit lifecycle or the CAAS substrate",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	JujuExternalHostNameKey: {
		Description: "the external hostname of an exposed application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	JujuApplicationPath: {
		Description: "the relative http path used to access an application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}

// ConfigSchema returns the valid fields for a CAAS application config.
func ConfigSchema(providerFields environschema.Fields) (environschema.Fields, error) {
	fields, err := configSchema(providerFields)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return fields, nil
}

func configSchema(extra environschema.Fields) (environschema.Fields, error) {
	fields := make(environschema.Fields)
	for name, field := range configFields {
		fields[name] = field
	}
	for name, field := range extra {
		if _, ok := configFields[name]; ok {
			return nil, errors.Errorf("config field %q clashes with common config", name)
		}
		fields[name] = field
	}
	return fields, nil
}

// ConfigDefaults returns the default values for a CAAS application config.
func ConfigDefaults(providerDefaults schema.Defaults) schema.Defaults {
	defaults := schema.Defaults{
		JujuApplicationPath: JujuDefaultApplicationPath,
		JujuManagedUnits:    JujuDefaultJujuManagedUnits,
	}
	for key, value := range providerDefaults {
		defaults[key] = value
	}
	return defaults
}
