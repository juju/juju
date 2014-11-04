// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"github.com/juju/juju/version"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.upgrade")

// Operation defines what steps to perform to upgrade to a target version.
type Operation interface {
	// The Juju version for which this operation is applicable.
	// Upgrade operations designed for versions of Juju earlier
	// than we are upgrading from are not run since such steps would
	// already have been used to get to the version we are running now.
	TargetVersion() version.Number

	// State requiring steps to perform during an upgrade. These go first.
	StateSteps() []StateStep

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
	stateSteps    []StateStep
	steps         []Step
}

// StateSteps is defined on the Operation interface.
func (u upgradeToVersion) StateSteps() []StateStep {
	return u.stateSteps
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
	return newUpgradeOpsIterator(from, version.Current.Number).Next()
}

// PerformUpgrade runs the business logic needed to upgrade the current "from" version to this
// version of Juju on the "target" type of machine.
func PerformUpgrade(from version.Number, target Target, context Context) error {
	if isStateTarget(target) {
		if err := runUpgradeSteps(from, target, getStateSteps, runStateStep, context); err != nil {
			return err
		}
	}
	if err := runUpgradeSteps(from, target, getSteps, runStep, context); err != nil {
		return err
	}
	logger.Infof("All upgrade steps completed successfully")
	return nil
}

func isStateTarget(target Target) bool {
	return target == StateServer || target == DatabaseMaster
}

// runUpgradeSteps finds all the upgrade operations relevant to
// target. The steps for each operation are retrieved by calling the
// passed getSteps function on the operation and are run by calling
// runStep.
//
// As soon as any error is encountered, the operation is aborted since
// subsequent steps may required successful completion of earlier
// ones. The steps must be idempotent so that the entire upgrade
// operation can be retried.
func runUpgradeSteps(
	from version.Number,
	target Target,
	getSteps func(Operation) []GenericStep,
	runStep func(Context, GenericStep) error,
	context Context,
) error {
	for ops := newUpgradeOpsIterator(from, version.Current.Number); ops.Next(); {
		steps := getSteps(ops.Get())
		for _, step := range steps {
			if !validTarget(target, step.Targets()) {
				continue
			}
			logger.Infof("running upgrade step on target %q: %v", target, step.Description())
			if err := runStep(context, step); err != nil {
				logger.Errorf("state upgrade step %q failed: %v", step.Description(), err)
				return &upgradeError{
					description: step.Description(),
					err:         err,
				}
			}
		}

	}
	return nil
}

func getStateSteps(op Operation) (steps []GenericStep) {
	for _, step := range op.StateSteps() {
		steps = append(steps, step)
	}
	return
}

func runStateStep(context Context, step GenericStep) error {
	return step.(StateStep).Run(context.(StateContext))
}

func getSteps(op Operation) (steps []GenericStep) {
	for _, step := range op.Steps() {
		steps = append(steps, step)
	}
	return
}

func runStep(context Context, step GenericStep) error {
	return step.(Step).Run(context)
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

type upgradeOpsIterator struct {
	from    version.Number
	to      version.Number
	allOps  []Operation
	current int
}

func newUpgradeOpsIterator(from, to version.Number) *upgradeOpsIterator {
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
		allOps:  upgradeOperations(),
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
