// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"

	"github.com/juju/juju/domain/charm"
)

func decodeConfig(configs []charmConfig) (charm.Config, error) {
	result := charm.Config{
		Options: make(map[string]charm.Option),
	}
	for _, config := range configs {
		optionType, err := decodeConfigType(config.Type)
		if err != nil {
			return charm.Config{}, fmt.Errorf("cannot decode config type %q: %w", config.Type, err)
		}

		defaultValue, err := decodeConfigDefaultValue(optionType, config.DefaultValue)
		if err != nil {
			return charm.Config{}, fmt.Errorf("cannot decode config default value %q: %w", config.DefaultValue, err)
		}

		result.Options[config.Key] = charm.Option{
			Type:        optionType,
			Description: config.Description,
			Default:     defaultValue,
		}
	}
	return result, nil
}

func decodeConfigType(t string) (charm.OptionType, error) {
	switch t {
	case "string":
		return charm.OptionString, nil
	case "int":
		return charm.OptionInt, nil
	case "float":
		return charm.OptionFloat, nil
	case "boolean":
		return charm.OptionBool, nil
	case "secret":
		return charm.OptionSecret, nil
	default:
		return "", fmt.Errorf("unknown config type %q", t)
	}
}

func decodeConfigDefaultValue(t charm.OptionType, value string) (any, error) {
	switch t {
	case charm.OptionString, charm.OptionSecret:
		return value, nil
	case charm.OptionInt:
		return strconv.Atoi(value)
	case charm.OptionFloat:
		return strconv.ParseFloat(value, 64)
	case charm.OptionBool:
		return strconv.ParseBool(value)
	default:
		return nil, fmt.Errorf("unknown config type %q", t)
	}
}
