// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

//UnitSeriesUpgradeStatus is the current status of a series upgrade for units
type UnitSeriesUpgradeStatus string

// Machine upgrade series status will be removed
type MachineSeriesUpgradeStatus = UnitSeriesUpgradeStatus

const (
	NotStarted       UnitSeriesUpgradeStatus = "NotStarted"
	PrepareStarted   UnitSeriesUpgradeStatus = "Prepare Started"
	PrepareMachine   UnitSeriesUpgradeStatus = "prepare machine"
	PrepareCompleted UnitSeriesUpgradeStatus = "Prepare Completed"
	CompleteStarted  UnitSeriesUpgradeStatus = "Complete Started"
	Completed        UnitSeriesUpgradeStatus = "Completed"
	UnitErrored      UnitSeriesUpgradeStatus = "Errored"
)

// Validates a string returning an UpgradeSeriesPrepareStatus, if valid, or an error.
func ValidateUnitSeriesUpgradeStatus(series string) (UnitSeriesUpgradeStatus, error) {
	unCheckedStatus := UnitSeriesUpgradeStatus(series)
	switch unCheckedStatus {
	case NotStarted, PrepareStarted, PrepareCompleted, CompleteStarted, Completed, UnitErrored:
		return unCheckedStatus, nil
	}

	return NotStarted, errors.Errorf("encountered invalid unit upgrade series status of %q", series)
}
