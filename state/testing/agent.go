// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.state.testing")

// SetAgentVersion sets the current agent version in the state's
// environment configuration.
func SetAgentVersion(st *state.State, vers version.Number) error {
	cfg, err := st.EnvironConfig()
	if err != nil {
		logger.Debugf("Failed to get EnvironConfig: %v", err)
		return err
	}
	existingVers, _ := cfg.AgentVersion()
	logger.Debugf("Setting AgentVersion from %q to %q", existingVers.String(), vers.String())
	cfg, err = cfg.Apply(map[string]interface{}{"agent-version": vers.String()})
	if err != nil {
		return err
	}
	return st.SetEnvironConfig(cfg)
}
