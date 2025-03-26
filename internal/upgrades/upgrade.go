// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/version"
)

var logger = internallogger.GetLogger("juju.upgrade")

// Step defines an idempotent operation that is run to perform
// a specific upgrade step.
type Step interface {
	// Description is a human readable description of what the upgrade step does.
	Description() string

	// Targets returns the target machine types for which the upgrade step is applicable.
	Targets() []Target

	// Run executes the upgrade business logic.
	Run(Context) error
}

// Operation defines what steps to perform to upgrade to a target version.
type Operation interface {
	// TargetVersion is the Juju version for which this operation is applicable.
	// Upgrade operations designed for versions of Juju earlier
	// than we are upgrading from are not run since such steps would
	// already have been used to get to the version we are running now.
	TargetVersion() version.Number

	// Steps to perform during an upgrade.
	Steps() []Step
}

// OperationSource provides a means of obtaining upgrade operations.
type OperationSource interface {
	// UpgradeOperations returns Operations to run during upgrade.
	UpgradeOperations() []Operation
}

// Target defines the type of machine for which a particular upgrade
// step can be run.
type Target string

const (
	// AllMachines applies to any machine.
	AllMachines = Target("allMachines")

	// HostMachine is a machine on which units are deployed.
	HostMachine = Target("hostMachine")

	// Controller is a machine participating in a Juju controller cluster.
	Controller = Target("controller")

	// DatabaseMaster is a Controller that has the master database, and as such
	// is the only target that should run database schema upgrade steps.
	DatabaseMaster = Target("databaseMaster")
)

// upgradeToVersion encapsulates the steps which need to be run to
// upgrade any prior version of Juju to targetVersion.
//
//nolint:unused
type upgradeToVersion struct {
	targetVersion version.Number
	steps         []Step
}

// Steps is defined on the Operation interface.
//
//nolint:unused
func (u upgradeToVersion) Steps() []Step {
	return u.steps
}

// TargetVersion is defined on the Operation interface.
//
//nolint:unused
func (u upgradeToVersion) TargetVersion() version.Number {
	return u.targetVersion
}

// upgradeError records a description of the step being performed and the error.
type upgradeError struct {
	description string
	err         error
}

func (e *upgradeError) Error() string {
	return fmt.Sprintf("%s: %v", e.description, e.err)
}

// UpgradeStepsFunc is the function type of Upgrade. This may be
// used to provide an alternative to Upgrade to the upgrade steps
// worker.
type UpgradeStepsFunc func(from version.Number, targets []Target, context Context) error

// PerformUpgradeSteps runs the business logic needed to upgrade the current
// "from" version to this version of Juju on the "target" type of machine.
func PerformUpgradeSteps(from version.Number, targets []Target, context Context) error {
	ops := newUpgradeOpsIterator(from)
	if err := runUpgradeSteps(ops, targets, context.APIContext()); err != nil {
		return errors.Trace(err)
	}
	logger.Infof(stdcontext.TODO(), "All upgrade steps completed successfully")
	return nil
}

// runUpgradeSteps finds all the upgrade operations relevant to
// the targets given and runs the associated upgrade steps.
//
// As soon as any error is encountered, the operation is aborted since
// subsequent steps may required successful completion of earlier
// ones. The steps must be idempotent so that the entire upgrade
// operation can be retried.
func runUpgradeSteps(ops *opsIterator, targets []Target, context Context) error {
	for ops.Next() {
		for _, step := range ops.Get().Steps() {
			if targetsMatch(targets, step.Targets()) {
				logger.Infof(stdcontext.TODO(), "running upgrade step: %v", step.Description())
				if err := step.Run(context); err != nil {
					logger.Errorf(stdcontext.TODO(), "upgrade step %q failed: %v", step.Description(), err)
					return &upgradeError{
						description: step.Description(),
						err:         err,
					}
				}
			}
		}
	}
	return nil
}

// targetsMatch returns true if any machineTargets match any of
// stepTargets.
func targetsMatch(machineTargets []Target, stepTargets []Target) bool {
	for _, machineTarget := range machineTargets {
		for _, stepTarget := range stepTargets {
			if machineTarget == stepTarget || stepTarget == AllMachines {
				return true
			}
		}
	}
	return false
}

// upgradeStep is a default Step implementation.
type upgradeStep struct {
	description string
	targets     []Target
	run         func(Context) error
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
func (step *upgradeStep) Run(context Context) error {
	return step.run(context)
}
