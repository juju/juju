// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
)

// Context provides the dependencies used when executing upgrade steps.
type Context interface {
	// APIState returns an API connection to state.
	//
	// TODO(mjs) - for 2.0, this should return a base.APICaller
	// instead of api.Connection once the 1.x upgrade steps have been
	// removed. Upgrade steps should not be able close the API
	// connection.
	APIState() api.Connection

	// State returns a connection to state. This will be non-nil
	// only in the context of a controller.
	State() StateBackend

	// AgentConfig returns the agent config for the machine that is being
	// upgraded.
	AgentConfig() agent.ConfigSetter

	// StateContext returns a new Context suitable for State-based
	// upgrade steps.
	StateContext() Context

	// APIContext returns a new Context suitable for API-based upgrade
	// steps.
	APIContext() Context
}

// NewContext returns a new upgrade context.
func NewContext(
	agentConfig agent.ConfigSetter,
	api api.Connection,
	st StateBackend,
) Context {
	return &upgradeContext{
		agentConfig: agentConfig,
		api:         api,
		st:          st,
	}
}

// upgradeContext is a default Context implementation.
type upgradeContext struct {
	agentConfig agent.ConfigSetter
	api         api.Connection
	st          StateBackend
}

// APIState is defined on the Context interface.
//
// This will panic if called on a Context returned by StateContext.
func (c *upgradeContext) APIState() api.Connection {
	if c.api == nil {
		panic("API not available from this context")
	}
	return c.api
}

// State is defined on the Context interface.
//
// This will panic if called on a Context returned by APIContext.
func (c *upgradeContext) State() StateBackend {
	if c.st == nil {
		panic("State not available from this context")
	}
	return c.st
}

// AgentConfig is defined on the Context interface.
func (c *upgradeContext) AgentConfig() agent.ConfigSetter {
	return c.agentConfig
}

// StateContext is defined on the Context interface.
func (c *upgradeContext) StateContext() Context {
	return &upgradeContext{
		agentConfig: c.agentConfig,
		st:          c.st,
	}
}

// APIContext is defined on the Context interface.
func (c *upgradeContext) APIContext() Context {
	return &upgradeContext{
		agentConfig: c.agentConfig,
		api:         c.api,
	}
}
