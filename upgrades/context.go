// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/state"
)

// Context is used give the upgrade steps that interact with the API
// what they need to do their job.
type Context interface {
	// APIState returns an API connection to state.
	APIState() *api.State

	// State returns a connection to state. This will be non-nil
	// only in the context of a state server.
	State() *state.State

	// AgentConfig returns the agent config for the machine that is being
	// upgraded.
	AgentConfig() agent.ConfigSetter
}

// NewContext returns a new upgrade context.
func NewContext(agentConfig agent.ConfigSetter, api *api.State, st *state.State) Context {
	return &upgradeContext{
		agentConfig: agentConfig,
		api:         api,
		st:          st,
	}
}

// upgradeContext is a default Context implementation.
type upgradeContext struct {
	agentConfig agent.ConfigSetter
	api         *api.State
	st          *state.State
}

// APIState is defined on the Context interface.
func (c *upgradeContext) APIState() *api.State {
	return c.api
}

// State is defined on the Context interface.
func (c *upgradeContext) State() *state.State {
	return c.st
}

// AgentConfig is defined on the Context interface.
func (c *upgradeContext) AgentConfig() agent.ConfigSetter {
	return c.agentConfig
}
