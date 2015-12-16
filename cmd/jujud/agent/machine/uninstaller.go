// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// uninstallerManifoldConfig provides the dependencies for the
// uninstaller manifold.
type uninstallerManifoldConfig struct {
	AgentName          string
	APICallerName      string
	WriteUninstallFile func() error
}

// uninstallerManifold defines a simple start function which retrieves
// some dependencies, checks if the machine is dead and causes the
// agent to uninstall itself if it is. This doubles up on part of the
// machiner's functionality but the machiner doesn't run until
// upgrades are complete, and the upgrade related workers may not be
// able to make API requests if the machine is dead.
func uninstallerManifold(config uninstallerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if config.WriteUninstallFile == nil {
				return nil, errors.New("WriteUninstallFile not specified")
			}

			// Get the agent.
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}

			// Grab the tag and ensure that it's for a machine.
			tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
			if !ok {
				return nil, errors.New("agent's tag is not a machine tag")
			}

			// Get API connection.
			//
			// TODO(mjs) - this should really be a base.APICaller to
			// remove the possibility of the API connection being closed
			// here.
			var apiConn api.Connection
			if err := getResource(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}

			// Check if the machine is dead and set the agent to
			// uninstall if it is.
			//
			// TODO(mjs) - ideally this would be using its own facade.
			machine, err := apiConn.Agent().Entity(tag)
			if err != nil {
				return nil, err
			}
			if machine.Life() == params.Dead {
				if err := config.WriteUninstallFile(); err != nil {
					return nil, errors.Annotate(err, "writing uninstall agent file")
				}
				return nil, worker.ErrTerminateAgent
			}

			// All is well - we're done (no actual worker is actually returned).
			return nil, dependency.ErrUninstall
		},
	}
}
