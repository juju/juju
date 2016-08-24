// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/schema"
)

// These constants define named sources of model config attributes.
// After a call to UpdateModelConfig, any attributes added/removed
// will have a source of JujuModelConfigSource.
const (
	// JujuDefaultSource is used to label model config attributes that
	// come from hard coded defaults.
	JujuDefaultSource = "default"

	// JujuControllerSource is used to label model config attributes that
	// come from those associated with the controller.
	JujuControllerSource = "controller"

	// JujuModelConfigSource is used to label model config attributes that
	// have been explicitly set by the user.
	JujuModelConfigSource = "model"
)

// ConfigValue encapsulates a configuration
// value and its source.
type ConfigValue struct {
	// Value is the configuration value.
	Value interface{}

	// Source is the name of the inherited config
	// source from where the value originates.
	Source string
}

// ConfigValues is a map of configuration values keyed by attribute name.
type ConfigValues map[string]ConfigValue

// AllAttrs returns just the attribute values from the config.
func (c ConfigValues) AllAttrs() map[string]interface{} {
	result := make(map[string]interface{})
	for attr, val := range c {
		result[attr] = val
	}
	return result
}

// ConfigSchemaSource instances provide information on config attributes
// and the default attribute values.
type ConfigSchemaSource interface {
	// ConfigSchema returns extra config attributes specific
	// to this provider only.
	ConfigSchema() schema.Fields

	// ConfigDefaults returns the default values for the
	// provider specific config attributes.
	ConfigDefaults() schema.Defaults
}
