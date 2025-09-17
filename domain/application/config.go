// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
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
