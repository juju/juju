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
		upgradeToVersion{version.MustParse("2.0.0"), stateStepsFor20()},
		upgradeToVersion{version.MustParse("2.1.0"), stateStepsFor21()},
		upgradeToVersion{version.MustParse("2.2.0"), stateStepsFor22()},
		upgradeToVersion{version.MustParse("2.2.1"), stateStepsFor221()},
		upgradeToVersion{version.MustParse("2.2.2"), stateStepsFor222()},
		upgradeToVersion{version.MustParse("2.2.3"), stateStepsFor223()},
		upgradeToVersion{version.MustParse("2.3.0"), stateStepsFor23()},
		upgradeToVersion{version.MustParse("2.3.1"), stateStepsFor231()},
		upgradeToVersion{version.MustParse("2.3.2"), stateStepsFor232()},
		upgradeToVersion{version.MustParse("2.3.4"), stateStepsFor234()},
		upgradeToVersion{version.MustParse("2.3.6"), stateStepsFor236()},
		upgradeToVersion{version.MustParse("2.3.7"), stateStepsFor237()},
		upgradeToVersion{version.MustParse("2.4.0"), stateStepsFor24()},
		upgradeToVersion{version.MustParse("2.5.0"), stateStepsFor25()},
		upgradeToVersion{version.MustParse("2.5.3"), stateStepsFor253()},
		upgradeToVersion{version.MustParse("2.5.4"), stateStepsFor254()},
		upgradeToVersion{version.MustParse("2.6.0"), stateStepsFor26()},
		upgradeToVersion{version.MustParse("2.6.3"), stateStepsFor263()},
		upgradeToVersion{version.MustParse("2.6.5"), stateStepsFor265()},
		upgradeToVersion{version.MustParse("2.7.0"), stateStepsFor27()},
		upgradeToVersion{version.MustParse("2.7.7"), stateStepsFor277()},
		upgradeToVersion{version.MustParse("2.8.0"), stateStepsFor28()},
		upgradeToVersion{version.MustParse("2.8.1"), stateStepsFor281()},
		upgradeToVersion{version.MustParse("2.8.2"), stateStepsFor282()},
		upgradeToVersion{version.MustParse("2.8.6"), stateStepsFor286()},
		upgradeToVersion{version.MustParse("2.8.9"), stateStepsFor289()},
		upgradeToVersion{version.MustParse("2.9.0"), stateStepsFor29()},
		upgradeToVersion{version.MustParse("2.9.5"), stateStepsFor295()},
	}
	return steps
}

// upgradeOperations returns an ordered slice of sets of API-based
// operations needed to upgrade Juju to particular version. As per the
// state-based operations above, ordering is important.
var upgradeOperations = func() []Operation {
	steps := []Operation{
		upgradeToVersion{version.MustParse("2.0.0"), stepsFor20()},
		upgradeToVersion{version.MustParse("2.2.0"), stepsFor22()},
		upgradeToVersion{version.MustParse("2.4.0"), stepsFor24()},
		upgradeToVersion{version.MustParse("2.4.5"), stepsFor245()},
		upgradeToVersion{version.MustParse("2.6.3"), stepsFor263()},
		upgradeToVersion{version.MustParse("2.7.0"), stepsFor27()},
		upgradeToVersion{version.MustParse("2.7.2"), stepsFor272()},
		upgradeToVersion{version.MustParse("2.7.6"), stepsFor276()},
		upgradeToVersion{version.MustParse("2.8.0"), stepsFor28()},
		upgradeToVersion{version.MustParse("2.9.0"), stepsFor29()},
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
	// If from is not known, it is 1.16.
	if from == version.Zero {
		from = version.MustParse("1.16.0")
	}

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
