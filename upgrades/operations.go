// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/version/v2"

	jujuversion "github.com/juju/juju/version"
)

// stateUpgradeOperations returns an ordered slice of sets of
// state-based operations needed to upgrade Juju to particular
// version. The slice is ordered by target version, so that the sets
// of operations are executed in order from oldest version to most
// recent.
//
// All state-based operations are run before API-based operations
// (below).
var stateUpgradeOperations = func() []Operation {
	steps := []Operation{
		upgradeToVersion{version.Zero, stateStepsForSSTXN()},
		upgradeToVersion{version.MustParse("2.9.5"), stateStepsFor295()},
		upgradeToVersion{version.MustParse("2.9.6"), stateStepsFor296()},
		upgradeToVersion{version.MustParse("2.9.9"), stateStepsFor299()},
		upgradeToVersion{version.MustParse("2.9.10"), stateStepsFor2910()},
		upgradeToVersion{version.MustParse("2.9.12"), stateStepsFor2912()},
		upgradeToVersion{version.MustParse("2.9.15"), stateStepsFor2915()},
		upgradeToVersion{version.MustParse("2.9.17"), stateStepsFor2917()},
		upgradeToVersion{version.MustParse("2.9.19"), stateStepsFor2919()},
		upgradeToVersion{version.MustParse("2.9.20"), stateStepsFor2920()},
		upgradeToVersion{version.MustParse("2.9.22"), stateStepsFor2922()},
		upgradeToVersion{version.MustParse("2.9.24"), stateStepsFor2924()},
		upgradeToVersion{version.MustParse("2.9.26"), stateStepsFor2926()},
		upgradeToVersion{version.MustParse("2.9.29"), stateStepsFor2929()},
		upgradeToVersion{version.MustParse("2.9.30"), stateStepsFor2930()},
		upgradeToVersion{version.MustParse("2.9.32"), stateStepsFor2932()},
		upgradeToVersion{version.MustParse("2.9.33"), stateStepsFor2933()},
		upgradeToVersion{version.MustParse("3.0.0"), stateStepsFor30()},
	}
	return steps
}

// upgradeOperations returns an ordered slice of sets of API-based
// operations needed to upgrade Juju to particular version. As per the
// state-based operations above, ordering is important.
var upgradeOperations = func() []Operation {
	steps := []Operation{
		upgradeToVersion{version.MustParse("3.0.0"), stepsFor30()},
	}
	return steps
}

type opsIterator struct {
	from    version.Number
	to      version.Number
	allOps  []Operation
	current int
}

func newStateUpgradeOpsIterator(from version.Number) *opsIterator {
	return newOpsIterator(from, jujuversion.Current, stateUpgradeOperations())
}

func newUpgradeOpsIterator(from version.Number) *opsIterator {
	return newOpsIterator(from, jujuversion.Current, upgradeOperations())
}

func newOpsIterator(from, to version.Number, ops []Operation) *opsIterator {
	// Clear the version tag of the target release to ensure that all
	// upgrade steps for the release are run for alpha and beta
	// releases.
	// ...but only do this if the agent version has actually changed,
	// lest we trigger upgrade mode unnecessarily for non-final
	// versions.
	if from.Compare(to) != 0 {
		to.Tag = ""
	}

	return &opsIterator{
		from:    from,
		to:      to,
		allOps:  ops,
		current: -1,
	}
}

func (it *opsIterator) Next() bool {
	for {
		it.current++
		if it.current >= len(it.allOps) {
			return false
		}
		targetVersion := it.allOps[it.current].TargetVersion()

		// Always run zero version steps.
		if targetVersion.Compare(version.Zero) == 0 {
			return true
		}

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

func (it *opsIterator) Get() Operation {
	return it.allOps[it.current]
}
