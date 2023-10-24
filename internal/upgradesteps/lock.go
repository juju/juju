// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/worker/gate"
)

const (
	// ErrUpgradeStepsInvalidState is returned when the upgrade state is
	// invalid. We expect it to be in the db completed state, if that's not the
	// case, we can't proceed.
	ErrUpgradeStepsInvalidState = errors.ConstError("invalid upgrade state")

	// ErrFailedUpgradeSteps is returned when either controller fails to
	// complete its upgrade steps.
	ErrFailedUpgradeSteps = errors.ConstError("failed upgrade steps")

	// ErrUpgradeTimeout is returned when the upgrade steps fail to complete
	// within the timeout.
	ErrUpgradeTimeout = errors.ConstError("upgrade timeout")

	// defaultUpgradeTimeout is the default timeout for the upgrade to complete.
	// 10 minutes should be enough for the upgrade steps to complete.
	DefaultUpgradeTimeout = 10 * time.Minute

	DefaultRetryDelay    = 2 * time.Minute
	DefaultRetryAttempts = 5
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
