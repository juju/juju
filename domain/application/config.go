// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// TODO(dqlite) - we don't want to reference environs/config here but need a place for config key names.
const (
	// StorageDefaultBlockSourceKey is the key for the default block storage source.
	StorageDefaultBlockSourceKey = "storage-default-block-source"

	// StorageDefaultFilesystemSourceKey is the key for the default filesystem storage source.
	StorageDefaultFilesystemSourceKey = "storage-default-filesystem-source"
)

// DecodeConfig decodes the domain charm config representation into the internal
// charm config representation.
func DecodeConfig(options charm.Config) (internalcharm.ConfigSpec, error) {
	if len(options.Options) == 0 {
		return internalcharm.ConfigSpec{}, nil
	}

	result := make(map[string]internalcharm.Option)
	for name, option := range options.Options {
		opt, err := decodeConfigOption(option)
		if err != nil {
			return internalcharm.ConfigSpec{}, errors.Errorf("decode config option: %w", err)
		}

		result[name] = opt
	}
	return internalcharm.ConfigSpec{
		Options: result,
	}, nil
}

func decodeConfigOption(option charm.Option) (internalcharm.Option, error) {
	t, err := decodeOptionType(option.Type)
	if err != nil {
		return internalcharm.Option{}, errors.Errorf("decode option type: %w", err)
	}

	return internalcharm.Option{
		Type:        t,
		Description: option.Description,
		Default:     option.Default,
	}, nil
}

func decodeOptionType(t charm.OptionType) (string, error) {
	switch t {
	case charm.OptionString:
		return "string", nil
	case charm.OptionInt:
		return "int", nil
	case charm.OptionFloat:
		return "float", nil
	case charm.OptionBool:
		return "boolean", nil
	case charm.OptionSecret:
		return "secret", nil
	default:
		return "", errors.Errorf("unknown option type %q", t)
	}
}

// EncodeConfig encodes the internal charm config representation into the domain
// charm config representation.
func EncodeConfig(config *internalcharm.ConfigSpec) (charm.Config, error) {
	if config == nil || len(config.Options) == 0 {
		return charm.Config{}, nil
	}

	result := make(map[string]charm.Option)
	for name, option := range config.Options {
		opt, err := encodeConfigOption(option)
		if err != nil {
			return charm.Config{}, errors.Errorf("encode config option: %w", err)
		}

		result[name] = opt
	}
	return charm.Config{
		Options: result,
	}, nil
}

func encodeConfigOption(option internalcharm.Option) (charm.Option, error) {
	t, err := encodeOptionType(option.Type)
	if err != nil {
		return charm.Option{}, errors.Errorf("encode option type: %w", err)
	}

	return charm.Option{
		Type:        t,
		Description: option.Description,
		Default:     option.Default,
	}, nil
}

func encodeOptionType(t string) (charm.OptionType, error) {
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
		return "", errors.Errorf("unknown option type %q", t)
	}
}

// EncodeApplicationConfig encodes the application config into a map of
// AddApplicationConfig, which includes the type of the option.
// If there is no config, it returns nil.
// An error is returned if the config contains an option that is not in the
// charm config.
func EncodeApplicationConfig(config internalcharm.Config, charmConfig charm.Config) (map[string]AddApplicationConfig, error) {
	// If there is no config, then we can just return nil.
	if len(config) == 0 {
		return nil, nil
	}
	// The encoded config is the application config, with the type of the
	// option. Encoding the type ensures that if the type changes during an
	// upgrade, we can prevent a runtime error during that phase.
	encodedConfig := make(map[string]AddApplicationConfig, len(config))
	for k, v := range config {
		option, ok := charmConfig.Options[k]
		if !ok {
			return nil, errors.Errorf("missing charm config, expected %q", k)
		}
		encodedV, err := encodeApplicationConfigValue(v, option.Type)
		if err != nil {
			return nil, errors.Errorf("encoding config value for %q: %w", k, err)
		}
		encodedConfig[k] = AddApplicationConfig{
			Value: encodedV,
			Type:  option.Type,
		}
	}
	return encodedConfig, nil
}

// encodeApplicationConfigValue encodes an application config value to a string.
func encodeApplicationConfigValue(value any, vType charm.OptionType) (string, error) {
	switch vType {
	case charm.OptionString, charm.OptionSecret:
		str, ok := value.(string)
		if !ok {
			return "", errors.Errorf("expected string value, got %T", value)
		}
		return str, nil
	case charm.OptionInt:
		i, ok := value.(int64)
		if !ok {
			return "", errors.Errorf("expected int64 value, got %T", value)
		}
		return strconv.FormatInt(i, 10), nil
	case charm.OptionFloat:
		f, ok := value.(float64)
		if !ok {
			return "", errors.Errorf("expected float64 value, got %T", value)
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case charm.OptionBool:
		b, ok := value.(bool)
		if !ok {
			return "", errors.Errorf("expected bool value, got %T", value)
		}
		return strconv.FormatBool(b), nil
	default:
		return "", errors.Errorf("unsupported option type %q", vType)
	}
}
