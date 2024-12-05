// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

func decodeConfig(configs []charmConfig) (charm.Config, error) {
	result := charm.Config{
		Options: make(map[string]charm.Option),
	}
	for _, config := range configs {
		optionType, err := decodeConfigType(config.Type)
		if err != nil {
			return charm.Config{}, errors.Errorf("cannot decode config type %q: %w", config.Type, err)
		}

		defaultValue, err := decodeConfigDefaultValue(optionType, config.DefaultValue)
		if err != nil {
			return charm.Config{}, errors.Errorf("cannot decode config default value %v: %w", config.DefaultValue, err)
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
		return "", errors.Errorf("unknown config type %q", t)
	}
}

func decodeConfigDefaultValue(t charm.OptionType, value *string) (any, error) {
	if value == nil {
		return nil, nil
	}

	switch t {
	case charm.OptionString, charm.OptionSecret:
		return *value, nil
	case charm.OptionInt:
		return strconv.Atoi(*value)
	case charm.OptionFloat:
		return strconv.ParseFloat(*value, 64)
	case charm.OptionBool:
		return strconv.ParseBool(*value)
	default:
		return nil, errors.Errorf("unknown config type %q", t)
	}
}

func encodeConfig(id corecharm.ID, config charm.Config) ([]setCharmConfig, error) {
	result := make([]setCharmConfig, 0, len(config.Options))
	for key, option := range config.Options {
		encodedType, err := encodeConfigType(option.Type)
		if err != nil {
			return nil, errors.Errorf("cannot encode config type %q: %w", option.Type, err)
		}

		encodedDefaultValue, err := encodeConfigDefaultValue(option.Default)
		if err != nil {
			return nil, errors.Errorf("cannot encode config default value %q: %w", option.Default, err)
		}

		result = append(result, setCharmConfig{
			CharmUUID:    id.String(),
			Key:          key,
			TypeID:       encodedType,
			Description:  option.Description,
			DefaultValue: encodedDefaultValue,
		})
	}
	return result, nil
}

func encodeConfigType(t charm.OptionType) (int, error) {
	switch t {
	case charm.OptionString:
		return 0, nil
	case charm.OptionInt:
		return 1, nil
	case charm.OptionFloat:
		return 2, nil
	case charm.OptionBool:
		return 3, nil
	case charm.OptionSecret:
		return 4, nil
	default:
		return -1, errors.Errorf("unknown config type %q", t)
	}
}

func encodeConfigDefaultValue(value any) (*string, error) {
	switch v := value.(type) {
	case string:
		return ptr(v), nil
	case int:
		return ptr(strconv.Itoa(v)), nil
	case int64:
		return ptr(strconv.FormatInt(v, 10)), nil
	case float64:
		return ptr(strconv.FormatFloat(v, 'f', -1, 64)), nil
	case bool:
		return ptr(strconv.FormatBool(v)), nil
	case nil:
		return nil, nil
	default:
		return nil, errors.Errorf("unknown config default value type %T", value)
	}
}

func ptr[T any](v T) *T {
	return &v
}
