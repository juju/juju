// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
)

// StateStep defines an idempotent operation that is run to perform
// a specific upgrade step that interacts with State directly..
type StateStep interface {
	// Description is a human readable description of what the upgrade step does.
	Description() string

	// Targets returns the target machine types for which the upgrade step is applicable.
	Targets() []Target

	// Run executes the upgrade business logic.
	Run(StateContext) error
}

// StateContext is used give upgrade steps that need to interact
// directly with state what they need to do their job.
type StateContext interface {
	// State returns a connection to state.
	State() *state.State

	// AgentConfig returns the agent config for the machine that is being
	// upgraded.
	AgentConfig() agent.ConfigSetter
}

// stateUpgradeStep is a default StateStep implementation.
type stateUpgradeStep struct {
	description string
	targets     []Target
	run         func(StateContext) error
}

// Description is defined on the StateStep interface.
func (step *stateUpgradeStep) Description() string {
	return step.description
}

// Targets is defined on the StateStep interface.
func (step *stateUpgradeStep) Targets() []Target {
	return step.targets
}

// Run is defined on the StateStep interface.
func (step *stateUpgradeStep) Run(context StateContext) error {
	return step.run(context)
}
