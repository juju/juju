// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/internal/worker/gate"
)

// Version encapsulates the version of Juju that the agent has upgraded to.
// This is used to identify the current version.
type Version interface {
	UpgradedToVersion() version.Number
}

// NewLock creates a gate.Lock to be used to synchronise workers which
// need to start after upgrades have completed. The returned Lock should
// be passed to NewWorker. If the agent has already upgraded to the
// current version, then the lock will be returned in the released state.
func NewLock(previous Version, current version.Number) gate.Lock {
	lock := gate.NewLock()

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := previous.UpgradedToVersion().ToPatch()
	if upgradedToVersion == current.ToPatch() {
		lock.Unlock()
	}

	return lock
}
