// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"
)

// TrustConfigOptionName is the option name used to set trust level in application configuration.
const TrustConfigOptionName = "trust"
const defaultTrustLevel = false

var trustFields = environschema.Fields{
	TrustConfigOptionName: {
		Description: "Does this application have access to trusted credentials",
		Type:        environschema.Tbool,
		Group:       environschema.JujuGroup,
	},
}

var trustDefaults = schema.Defaults{
	TrustConfigOptionName: defaultTrustLevel,
}

// AddTrustSchemaAndDefaults adds trust schema fields and defaults to an existing set of schema fields and defaults.
func AddTrustSchemaAndDefaults(schema environschema.Fields, defaults schema.Defaults) (environschema.Fields, schema.Defaults, error) {
	newSchema, err := addTrustSchema(schema)
	newDefaults := addTrustDefaults(defaults)
	return newSchema, newDefaults, err
}

func addTrustDefaults(defaults schema.Defaults) schema.Defaults {
	newDefaults := make(schema.Defaults)
	for key, value := range trustDefaults {
		newDefaults[key] = value
	}
	for key, value := range defaults {
		newDefaults[key] = value
	}
	return newDefaults
}

// [TODO](externalreality): This is copied from CAAS configuration code. This is
// a generic builder pattern and can likely be generalized to all application
// schema fields.
func addTrustSchema(extra environschema.Fields) (environschema.Fields, error) {
	fields := make(environschema.Fields)
	for name, field := range trustFields {
		fields[name] = field
	}
	for name, field := range extra {
		if _, ok := trustFields[name]; ok {
			return nil, errors.Errorf("config field %q clashes with common config", name)
		}
		fields[name] = field
	}
	return fields, nil
}
