// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"context"

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

	// JujuRegionSource is used to label model config attributes that come from
	// those associated with the region where the model is
	// running.
	JujuRegionSource = "region"

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

// ConfigSchemaSourceGetter is a type for getting a ConfigSchemaSource.
type ConfigSchemaSourceGetter func(context.Context, string) (ConfigSchemaSource, error)

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

// ModelDefaultAttributes is a map of configuration values to a list of possible
// values.
type ModelDefaultAttributes map[string]AttributeDefaultValues

// AttributeDefaultValues represents all the default values at each level for a given
// setting.
type AttributeDefaultValues struct {
	// Default and Controller represent the values as set at those levels.
	Default    interface{} `json:"default,omitempty" yaml:"default,omitempty"`
	Controller interface{} `json:"controller,omitempty" yaml:"controller,omitempty"`
	// Regions is a slice of Region representing the values as set in each
	// region.
	Regions []RegionDefaultValue `json:"regions,omitempty" yaml:"regions,omitempty"`
}

// RegionDefaultValue holds the region information for each region in DefaultSetting.
type RegionDefaultValue struct {
	// Name represents the region name for this specific setting.
	Name string `json:"name" yaml:"name"`
	// Value is the value of the setting this represents in the named region.
	Value interface{} `json:"value" yaml:"value"`
}
