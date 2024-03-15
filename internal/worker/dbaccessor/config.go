// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"os"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// ClusterConfig describes the ability to retrieve cluster configuration.
type ClusterConfig interface {
	// DBBindAddresses returns a map of addresses keyed by controller unit ID.
	DBBindAddresses() (map[string]string, error)
}

type controllerConfig struct {
	// DBBindAddresses is a map of addresses keyed by controller unit ID.
	DBBindAddresses map[string]string `yaml:"db-bind-addresses"`
}

// controllerConfigReader reads and parses controller
// configuration from a given file path.
// At the time if writing, this worker constitutes the only usages of
// charm-written controller configuration. Any expansion of its usage should
// be accompanied by a generalisation of this implementation along the lines
// of agent configuration.
type controllerConfigReader struct {
	configPath string
}

// DBBindAddresses returns a map of addresses keyed by controller unit ID.
func (c controllerConfigReader) DBBindAddresses() (map[string]string, error) {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return nil, errors.Annotatef(err, "reading config file from %s", c.configPath)
	}

	var cfg controllerConfig
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Annotate(err, "parsing config file contents")
	}

	return cfg.DBBindAddresses, nil
}
