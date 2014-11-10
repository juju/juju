// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"github.com/juju/juju/version"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.upgrade")

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
	// The Juju version for which this operation is applicable.
	// Upgrade operations designed for versions of Juju earlier
	// than we are upgrading from are not run since such steps would
	// already have been used to get to the version we are running now.
	TargetVersion() version.Number

	// Steps to perform during an upgrade.
	Steps() []Step
}

// Target defines the type of machine for which a particular upgrade
// step can be run.
type Target string

const (
	// AllMachines applies to any machine.
	AllMachines = Target("allMachines")

	// HostMachine is a machine on which units are deployed.
	HostMachine = Target("hostMachine")

	// StateServer is a machine participating in a Juju state server cluster.
	StateServer = Target("stateServer")

	// DatabaseMaster is a StateServer that has the master database, and as such
	// is the only target that should run database schema upgrade steps.
	DatabaseMaster = Target("databaseMaster")
)

// upgradeToVersion encapsulates the steps which need to be run to
// upgrade any prior version of Juju to targetVersion.
type upgradeToVersion struct {
	targetVersion version.Number
	steps         []Step
}

// Steps is defined on the Operation interface.
func (u upgradeToVersion) Steps() []Step {
	return u.steps
}

// TargetVersion is defined on the Operation interface.
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

// AreUpgradesDefined returns true if there are upgrade operations
// defined between the version supplied and the running software
// version.
func AreUpgradesDefined(from version.Number) bool {
	return newUpgradeOpsIterator(from, version.Current.Number, upgradeOperations()).Next() ||
		newUpgradeOpsIterator(from, version.Current.Number, stateUpgradeOperations()).Next()
}

// PerformUpgrade runs the business logic needed to upgrade the current "from" version to this
// version of Juju on the "target" type of machine.
func PerformUpgrade(from version.Number, target Target, context Context) error {
	if isStateTarget(target) {
		ops := newUpgradeOpsIterator(from, version.Current.Number, stateUpgradeOperations())
		if err := runUpgradeSteps(ops, target, context.StateContext()); err != nil {
			return err
		}
	}
	ops := newUpgradeOpsIterator(from, version.Current.Number, upgradeOperations())
	if err := runUpgradeSteps(ops, target, context.APIContext()); err != nil {
		return err
	}
	logger.Infof("All upgrade steps completed successfully")
	return nil
}

func isStateTarget(target Target) bool {
	return target == StateServer || target == DatabaseMaster
}

// runUpgradeSteps finds all the upgrade operations relevant to
// target and runs the associated upgrade steps.
//
// As soon as any error is encountered, the operation is aborted since
// subsequent steps may required successful completion of earlier
// ones. The steps must be idempotent so that the entire upgrade
// operation can be retried.
func runUpgradeSteps(ops *upgradeOpsIterator, target Target, context Context) error {
	for ops.Next() {
		for _, step := range ops.Get().Steps() {
			if !validTarget(target, step.Targets()) {
				continue
			}
			logger.Infof("running upgrade step on target %q: %v", target, step.Description())
			if err := step.Run(context); err != nil {
				logger.Errorf("upgrade step %q failed: %v", step.Description(), err)
				return &upgradeError{
					description: step.Description(),
					err:         err,
				}
			}
		}

	}
	return nil
}

// validTarget returns true if target matches targets.
func validTarget(target Target, targets []Target) bool {
	for _, opTarget := range targets {
		if opTarget == AllMachines || target == opTarget ||
			(opTarget == StateServer && target == DatabaseMaster) {
			return true
		}
	}
	return len(targets) == 0
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

type upgradeOpsIterator struct {
	from    version.Number
	to      version.Number
	allOps  []Operation
	current int
}

func newUpgradeOpsIterator(from, to version.Number, ops []Operation) *upgradeOpsIterator {
	// If from is not known, it is 1.16.
	if from == version.Zero {
		from = version.MustParse("1.16.0")
	}
	// Clear the version tag of the target release to ensure that all
	// upgrade steps for the release are run for alpha and beta releases.
	to.Tag = ""
	return &upgradeOpsIterator{
		from:    from,
		to:      to,
		allOps:  ops,
		current: -1,
	}
}

func (it *upgradeOpsIterator) Next() bool {
	for {
		it.current++
		if it.current >= len(it.allOps) {
			return false
		}
		targetVersion := it.allOps[it.current].TargetVersion()

		// Do not run steps for versions of Juju earlier or same as we are upgrading from.
		if targetVersion.Compare(it.from) <= 0 {
			continue
		}
		// Do not run steps for versions of Juju later than we are upgrading to.
		if targetVersion.Compare(it.to) > 0 {
			continue
		}
		return true
	}
}

func (it *upgradeOpsIterator) Get() Operation {
	return it.allOps[it.current]
}
