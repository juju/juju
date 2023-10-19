// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
)

// Context provides the dependencies used when executing upgrade steps.
type Context interface {
	// APIState returns an base APICaller to help make
	// an API connection to state.
	APIState() base.APICaller

	// AgentConfig returns the agent config for the machine that is being
	// upgraded.
	AgentConfig() agent.ConfigSetter

	// APIContext returns a new Context suitable for API-based upgrade
	// steps.
	APIContext() Context
}

// NewContext returns a new upgrade context.
func NewContext(
	agentConfig agent.ConfigSetter,
	apiCaller base.APICaller,
) Context {
	return &upgradeContext{
		agentConfig: agentConfig,
		apiCaller:   apiCaller,
	}
}

// upgradeContext is a default Context implementation.
type upgradeContext struct {
	agentConfig agent.ConfigSetter
	apiCaller   base.APICaller
}

// APIState is defined on the Context interface.
//
// This will panic if called on a Context returned by StateContext.
func (c *upgradeContext) APIState() base.APICaller {
	if c.apiCaller == nil {
		panic("API not available from this context")
	}
	return c.apiCaller
}

// AgentConfig is defined on the Context interface.
func (c *upgradeContext) AgentConfig() agent.ConfigSetter {
	return c.agentConfig
}

// APIContext is defined on the Context interface.
func (c *upgradeContext) APIContext() Context {
	return &upgradeContext{
		agentConfig: c.agentConfig,
		apiCaller:   c.apiCaller,
	}
}
