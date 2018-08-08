// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

// UpgradeSeriesStatus is a status that a machine or unit can be in during the
// execution of an OS series upgrade on its host machine.
type UpgradeSeriesStatus string

const (
	UpgradeSeriesNotStarted      UpgradeSeriesStatus = "not started"
	UpgradeSeriesPrepareStarted  UpgradeSeriesStatus = "prepare started"
	UpgradeSeriesPrepareMachine  UpgradeSeriesStatus = "prepare machine"
	UpgradeSeriesPrepareComplete UpgradeSeriesStatus = "prepare complete"
	UpgradeSeriesCompleteStarted UpgradeSeriesStatus = "complete started"
	UpgradeSeriesComplete        UpgradeSeriesStatus = "complete"
	UpgradeSeriesError           UpgradeSeriesStatus = "error"
)

//MachineSeriesUpgradeStatus is the current status a machine series upgrade
type MachineSeriesUpgradeStatus string

const (
	MachineSeriesUpgradeNotStarted    MachineSeriesUpgradeStatus = "NotStarted"
	MachineSeriesUpgradeStarted       MachineSeriesUpgradeStatus = "Started"
	MachineSeriesUpgradeAgentsStopped MachineSeriesUpgradeStatus = "AgentsStopped"
	MachineSeriesUpgradeComplete      MachineSeriesUpgradeStatus = "Complete"
)

//UnitSeriesUpgradeStatus is the current status of a series upgrade for units
type UnitSeriesUpgradeStatus string

const (
	NotStarted       UnitSeriesUpgradeStatus = "NotStarted"
	PrepareStarted   UnitSeriesUpgradeStatus = "Prepare Started"
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
