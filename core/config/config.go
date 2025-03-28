// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/schema"
	"gopkg.in/yaml.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/errors"
)

// ConfigAttributes represents config for an entity.
type ConfigAttributes map[string]interface{}

// Config encapsulates config for an entity.
type Config struct {
	attributes ConfigAttributes
	schema     configschema.Fields
	defaults   schema.Defaults
}

// NewConfig returns a new config instance with the given attributes and
// allowing for the extra provider attributes.
func NewConfig(attrs ConfigAttributes, schema configschema.Fields, defaults schema.Defaults) (*Config, error) {
	cfg := &Config{schema: schema, defaults: defaults}
	if err := cfg.setAttributes(attrs); err != nil {
		return nil, errors.Capture(err)
	}
	return cfg, nil
}

func (c *Config) setAttributes(attrs ConfigAttributes) error {
	checker, err := c.schemaChecker()
	if err != nil {
		return errors.Capture(err)
	}
	m := make(ConfigAttributes)
	for k, v := range attrs {
		m[k] = v
		field, ok := c.schema[k]
		if !ok || field.Type == configschema.Tstring {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var coerced interface{}
		err := yaml.Unmarshal([]byte(str), &coerced)
		if err != nil {
			return errors.Errorf(fmt.Sprintf("value %q for attribute %q not valid", str, k)+": %w", err).Add(coreerrors.NotValid)
		}
		m[k] = coerced
	}
	result, err := checker.Coerce(m, nil)
	if err != nil {
		return errors.Capture(err)
	}

	// Ensure that the underlying map is of the correct type, otherwise
	// this can panic.
	switch result := result.(type) {
	case map[string]interface{}:
		c.attributes = result
	case ConfigAttributes:
		c.attributes = result
	default:
		return errors.Errorf("unexpected result type %T", result)
	}

	return nil
}

// KnownConfigKeys returns the valid config keys.
func KnownConfigKeys(schema configschema.Fields) set.Strings {
	result := set.NewStrings()
	for name := range schema {
		result.Add(name)
	}
	return result
}

func (c *Config) schemaChecker() (schema.Checker, error) {
	schemaFields, schemaDefaults, err := c.schema.ValidationSchema()
	if err != nil {
		return nil, errors.Capture(err)
	}
	for key, value := range c.defaults {
		schemaDefaults[key] = value
	}
	return schema.StrictFieldMap(schemaFields, schemaDefaults), nil
}

// Validate returns an error if the config is not valid.
func (c *Config) Validate() error {
	return nil
}

// Attributes returns all the config attributes.
func (c *Config) Attributes() ConfigAttributes {
	if c == nil {
		return nil
	}
	result := make(ConfigAttributes)
	for k, v := range c.attributes {
		result[k] = v
	}
	return result
}

// Get gets the specified attribute.
func (c ConfigAttributes) Get(attrName string, defaultValue interface{}) interface{} {
	if val, ok := c[attrName]; ok {
		return val
	}
	return defaultValue
}

// GetBool gets the specified bool attribute.
func (c ConfigAttributes) GetBool(attrName string, defaultValue bool) bool {
	if val, ok := c[attrName]; ok {
		return val.(bool)
	}
	return defaultValue
}

// GetInt gets the specified int attribute.
func (c ConfigAttributes) GetInt(attrName string, defaultValue int) int {
	if val, ok := c[attrName]; ok {
		if value, ok := val.(float64); ok {
			return int(value)
		}
		return val.(int)
	}
	return defaultValue
}

// GetString gets the specified string attribute.
func (c ConfigAttributes) GetString(attrName string, defaultValue string) string {
	if val, ok := c[attrName]; ok {
		return val.(string)
	}
	return defaultValue
}

// GetStringMap gets the specified map attribute as map[string]string.
func (c ConfigAttributes) GetStringMap(attrName string, defaultValue map[string]string) (map[string]string, error) {
	if valData, ok := c[attrName]; ok {
		result := make(map[string]string)
		switch val := valData.(type) {
		case map[string]string:
			for k, v := range val {
				result[k] = v
			}
		case map[string]interface{}:
			for k, v := range val {
				result[k] = fmt.Sprintf("%v", v)
			}
		default:
			return nil, errors.Errorf("string map value of type %T %w", val, coreerrors.NotValid)
		}
		return result, nil
	}
	return defaultValue, nil
}
