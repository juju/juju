// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"strconv"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/schema"
)

// Settings is a group of charm config option names and values. A Settings
// S is considered valid by the Config C if every key in S is an option in
// C, and every value either has the correct type or is nil.
type Settings map[string]interface{}

// Option represents a single charm config option.
type Option struct {
	Type        string
	Description string
	Default     interface{}
}

// error replaces any supplied non-nil error with a new error describing a
// validation failure for the supplied value.
func (option Option) error(err *error, name string, value interface{}) {
	if *err != nil {
		*err = fmt.Errorf("option %q expected %s, got %#v", name, option.Type, value)
	}
}

// validate returns an appropriately-typed value for the supplied value, or
// returns an error if it cannot be converted to the correct type. Nil values
// are always considered valid.
func (option Option) validate(name string, value interface{}) (_ interface{}, err error) {
	if value == nil {
		return nil, nil
	}
	defer option.error(&err, name, value)
	if checker := optionTypeCheckers[option.Type]; checker != nil {
		if value, err = checker.Coerce(value, nil); err != nil {
			return nil, err
		}
		return value, nil
	}
	panic(fmt.Errorf("option %q has unknown type %q", name, option.Type))
}

var optionTypeCheckers = map[string]schema.Checker{
	"string":  schema.String(),
	"int":     schema.Int(),
	"float":   schema.Float(),
	"boolean": schema.Bool(),
}

// parse returns an appropriately-typed value for the supplied string, or
// returns an error if it cannot be parsed to the correct type.
func (option Option) parse(name, str string) (_ interface{}, err error) {
	defer option.error(&err, name, str)
	switch option.Type {
	case "string":
		return str, nil
	case "int":
		return strconv.ParseInt(str, 10, 64)
	case "float":
		return strconv.ParseFloat(str, 64)
	case "boolean":
		return strconv.ParseBool(str)
	}
	panic(fmt.Errorf("option %q has unknown type %q", name, option.Type))
}

// Config represents the supported configuration options for a charm,
// as declared in its config.yaml file.
type Config struct {
	Options map[string]Option
}

// NewConfig returns a new Config without any options.
func NewConfig() *Config {
	return &Config{map[string]Option{}}
}

// ReadConfig reads a Config in YAML format.
func ReadConfig(r io.Reader) (*Config, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var config *Config
	if err := goyaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("invalid config: empty configuration")
	}
	for name, option := range config.Options {
		switch option.Type {
		case "string", "int", "float", "boolean":
		case "":
			// Missing type is valid in python.
			option.Type = "string"
		default:
			return nil, fmt.Errorf("invalid config: option %q has unknown type %q", name, option.Type)
		}
		def := option.Default
		if def == "" && option.Type == "string" {
			// Skip normal validation for compatibility with pyjuju.
		} else if option.Default, err = option.validate(name, def); err != nil {
			option.error(&err, name, def)
			return nil, fmt.Errorf("invalid config default: %v", err)
		}
		config.Options[name] = option
	}
	return config, nil
}

// option returns the named option from the config, or an error if none
// such exists.
func (c *Config) option(name string) (Option, error) {
	if option, ok := c.Options[name]; ok {
		return option, nil
	}
	return Option{}, fmt.Errorf("unknown option %q", name)
}

// DefaultSettings returns settings containing the default value of every
// option in the config. Default values may be nil.
func (c *Config) DefaultSettings() Settings {
	out := make(Settings)
	for name, option := range c.Options {
		out[name] = option.Default
	}
	return out
}

// ValidateSettings returns a copy of the supplied settings with a consistent type
// for each value. It returns an error if the settings contain unknown keys
// or invalid values.
func (c *Config) ValidateSettings(settings Settings) (Settings, error) {
	out := make(Settings)
	for name, value := range settings {
		if option, err := c.option(name); err != nil {
			return nil, err
		} else if value, err = option.validate(name, value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

// FilterSettings returns the subset of the supplied settings that are valid.
func (c *Config) FilterSettings(settings Settings) Settings {
	out := make(Settings)
	for name, value := range settings {
		if option, err := c.option(name); err == nil {
			if value, err := option.validate(name, value); err == nil {
				out[name] = value
			}
		}
	}
	return out
}

// ParseSettingsStrings returns settings derived from the supplied map. Every
// value in the map must be parseable to the correct type for the option
// identified by its key. Empty values are interpreted as nil.
func (c *Config) ParseSettingsStrings(values map[string]string) (Settings, error) {
	out := make(Settings)
	for name, str := range values {
		option, err := c.option(name)
		if err != nil {
			return nil, err
		}
		value, err := option.parse(name, str)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

// ParseSettingsYAML returns settings derived from the supplied YAML data. The
// YAML must unmarshal to a map of strings to settings data; the supplied key
// must be present in the map, and must point to a map in which every value
// must have, or be a string parseable to, the correct type for the associated
// config option. Empty strings and nil values are both interpreted as nil.
func (c *Config) ParseSettingsYAML(yamlData []byte, key string) (Settings, error) {
	var allSettings map[string]Settings
	if err := goyaml.Unmarshal(yamlData, &allSettings); err != nil {
		return nil, fmt.Errorf("cannot parse settings data: %v", err)
	}
	settings, ok := allSettings[key]
	if !ok {
		return nil, fmt.Errorf("no settings found for %q", key)
	}
	out := make(Settings)
	for name, value := range settings {
		option, err := c.option(name)
		if err != nil {
			return nil, err
		}
		// Accept string values for compatibility with python.
		if str, ok := value.(string); ok {
			if value, err = option.parse(name, str); err != nil {
				return nil, err
			}
		} else if value, err = option.validate(name, value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}
