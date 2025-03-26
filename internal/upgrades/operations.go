// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/version"
)

// upgradeOperations returns an ordered slice of sets of API-based
// operations needed to upgrade Juju to particular version. As per the
// state-based operations above, ordering is important.
var upgradeOperations = func() []Operation {
	steps := []Operation{
		// Fill in when we have upgrade steps.
		upgradeToVersion{version.MustParse("6.6.6"), []Step(nil)},
	}
	return steps
}

type opsIterator struct {
	from    version.Number
	to      version.Number
	allOps  []Operation
	current int
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
