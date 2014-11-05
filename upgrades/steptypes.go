// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// GenericStep describes the interface shared between different types of upgrade steps.
type GenericStep interface {
	// Description is a human readable description of what the upgrade step does.
	Description() string

	// Targets returns the target machine types for which the upgrade step is applicable.
	Targets() []Target
}

// StateStep defines an idempotent operation that is run to perform
// a specific upgrade step that interacts with State directly..
type StateStep interface {
	GenericStep

	// Run executes the upgrade business logic.
	Run(StateContext) error
}

// Step defines an idempotent operation that is run to perform
// a specific upgrade step.
type Step interface {
	GenericStep

	// Run executes the upgrade business logic.
	Run(APIContext) error
}

// stateUpgradeStep is a default StateStep implementation.
type stateUpgradeStep struct {
	description string
	targets     []Target
	run         func(StateContext) error
}

var _ StateStep = (*stateUpgradeStep)(nil)

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

// upgradeStep is a default Step implementation.
type upgradeStep struct {
	description string
	targets     []Target
	run         func(APIContext) error
}

var _ Step = (*upgradeStep)(nil)

// Description is defined on the Step interface.
func (step *upgradeStep) Description() string {
	return step.description
}

// Targets is defined on the Step interface.
func (step *upgradeStep) Targets() []Target {
	return step.targets
}

// Run is defined on the Step interface.
func (step *upgradeStep) Run(context APIContext) error {
	return step.run(context)
}
