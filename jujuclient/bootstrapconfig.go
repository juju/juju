// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

// JujuBootstrapConfigPath is the location where bootstrap config is
// expected to be found.
func JujuBootstrapConfigPath() string {
	return osenv.JujuXDGDataHomePath("bootstrap-config.yaml")
}

// ReadBootstrapConfigFile loads all bootstrap configurations defined in a
// given file. If the file is not found, it is not an error.
func ReadBootstrapConfigFile(file string) (map[string]BootstrapConfig, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	configs, err := ParseBootstrapConfig(data)
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// WriteBootstrapConfigFile marshals to YAML details of the given bootstrap
// configurations and writes it to the bootstrap config file.
func WriteBootstrapConfigFile(configs map[string]BootstrapConfig) error {
	data, err := yaml.Marshal(bootstrapConfigCollection{configs})
	if err != nil {
		return errors.Annotate(err, "cannot marshal bootstrap configurations")
	}
	return utils.AtomicWriteFile(JujuBootstrapConfigPath(), data, os.FileMode(0600))
}

// ParseBootstrapConfig parses the given YAML bytes into bootstrap config
// metadata.
func ParseBootstrapConfig(data []byte) (map[string]BootstrapConfig, error) {
	var result bootstrapConfigCollection
	err := yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal bootstrap config")
	}
	return result.ControllerBootstrapConfig, nil
}

type bootstrapConfigCollection struct {
	ControllerBootstrapConfig map[string]BootstrapConfig `yaml:"controllers"`
}
