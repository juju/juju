// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plugin contains the code that interfaces with plugins for workload
// technologies such as Docker, Rocket, or systemd.
package plugin

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin/executable"
)

var logger = loggo.GetLogger("juju.workload.plugin")

// We special-case a specific name for the sake of the feature tests.
const testingPluginName = "testing-plugin"

// Find returns the plugin for the given name.
//
// If the plugin is not found then errors.NotFound is returned.
func Find(name, dataDir string) (workload.Plugin, error) {
	if name == testingPluginName {
		return find(name, dataDir)
	}

	plugin, err := findBuiltin(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return plugin, nil
}

// find returns the plugin for the given name. First it looks for an
// executable plugin and then it falls back to one of the built-in
// plugins. Favoring executable plugins allows charms to maintain
// control over the plugin's behavior.
//
// If the plugin is not found then errors.NotFound is returned.
func find(name, dataDir string) (workload.Plugin, error) {
	plugin, err := executable.FindPlugin(name, dataDir)
	if errors.IsNotFound(err) {
		plugin, err := findBuiltin(name)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return plugin, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return plugin, nil
}
