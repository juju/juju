// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package plugin contains the code that interfaces with plugins for workload
// technologies such as Docker, Rocket, or systemd.
package plugin

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
)

var logger = loggo.GetLogger("juju.workload.plugin")

// Find returns the plugin for the given name.
func Find(name, agentDir string) (workload.Plugin, error) {
	plugin, err := FindExecutablePlugin(name, agentDir)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return plugin, nil
}
