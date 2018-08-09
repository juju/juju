// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

//UpgradeSeriesStatus is the current status of a series upgrade for units
type UpgradeSeriesStatus string

// Machine upgrade series status will be removed
type MachineSeriesUpgradeStatus = UpgradeSeriesStatus

const (
	NotStarted       UpgradeSeriesStatus = "not started"
	PrepareStarted   UpgradeSeriesStatus = "prepare started"
	PrepareMachine   UpgradeSeriesStatus = "prepare machine"
	PrepareCompleted UpgradeSeriesStatus = "prepare completed"
	CompleteStarted  UpgradeSeriesStatus = "complete started"
	Completed        UpgradeSeriesStatus = "completed"
	UnitErrored      UpgradeSeriesStatus = "error"
)

// Validates a string returning an UpgradeSeriesPrepareStatus, if valid, or an error.
func ValidateUnitSeriesUpgradeStatus(series string) (UpgradeSeriesStatus, error) {
	unCheckedStatus := UpgradeSeriesStatus(series)
	switch unCheckedStatus {
	case NotStarted, PrepareStarted, PrepareCompleted, CompleteStarted, Completed, UnitErrored:
		return unCheckedStatus, nil
	}

	return NotStarted, errors.Errorf("encountered invalid unit upgrade series status of %q", series)
}
