// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import "github.com/juju/juju/version"

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
		upgradeToVersion{
			version.MustParse("1.18.0"),
			stateStepsFor118(),
		},
		upgradeToVersion{
			version.MustParse("1.21.0"),
			stateStepsFor121(),
		},
	}
	return steps
}

// upgradeOperations returns an ordered slice of sets of API-based
// operations needed to upgrade Juju to particular version. As per the
// state-based operations above, ordering is important.
var upgradeOperations = func() []Operation {
	steps := []Operation{
		upgradeToVersion{
			version.MustParse("1.18.0"),
			stepsFor118(),
		},
	}
	return steps
}
