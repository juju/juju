// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plugin contains the code that interfaces with plugins for workload
// technologies such as Docker, Rocket, or systemd.
package plugin

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin/docker"
)

var logger = loggo.GetLogger("juju.workload.plugin")

var builtinPlugins = map[string]workload.Plugin{
	"docker": docker.NewPlugin(),
}

// Find returns the plugin for the given name.
func Find(name, dataDir string) (workload.Plugin, error) {
	plugin, err := FindExecutablePlugin(name, dataDir)
	if errors.IsNotFound(err) {
		plugin, ok := builtinPlugins[name]
		if !ok {
			return nil, errors.NotFoundf("plugin %q", name)
		}
		return plugin, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	return plugin, nil
}
