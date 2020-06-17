// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/version"
	"github.com/juju/worker/v2/dependency"
)

// manifoldsConfig allows specialisation of the result of Manifolds.
type manifoldsConfig struct {
	// TODO

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// PreviousAgentVersion passes through the version the unit
	// agent was running before the current restart.
	PreviousAgentVersion version.Number
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a k8s agent unit command. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config manifoldsConfig) dependency.Manifolds {
	// TODO
	return dependency.Manifolds{}
}
