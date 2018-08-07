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

// TODO (manadart 2018-08-07) Everything below here to be discarded.

// ToOldStatus is a shim between the new and old status representations.
// It exists to bootstrap that transition. Unit tests omitted by design.
func (s UpgradeSeriesStatus) ToOldStatus() (UpgradeSeriesStatusType, MachineSeriesUpgradeStatus) {
	switch s {
	case UpgradeSeriesNotStarted:
		return PrepareStatus, MachineSeriesUpgradeNotStarted
	case UpgradeSeriesPrepareStarted:
		return PrepareStatus, MachineSeriesUpgradeStarted
	case UpgradeSeriesPrepareMachine:
		return PrepareStatus, MachineSeriesUpgradeAgentsStopped
	case UpgradeSeriesPrepareComplete:
		return PrepareStatus, MachineSeriesUpgradeComplete
	case UpgradeSeriesCompleteStarted:
		return CompleteStatus, MachineSeriesUpgradeStarted
	case UpgradeSeriesComplete:
		return CompleteStatus, MachineSeriesUpgradeComplete
	default:
		return "", ""
	}
}

func FromOldUpgradeSeriesStatus(prepare, complete MachineSeriesUpgradeStatus) (UpgradeSeriesStatus, error) {
	switch prepare {
	case MachineSeriesUpgradeNotStarted:
		return UpgradeSeriesNotStarted, nil
	case MachineSeriesUpgradeStarted:
		return UpgradeSeriesPrepareStarted, nil
	case MachineSeriesUpgradeAgentsStopped:
		return UpgradeSeriesPrepareMachine, nil
	}

	// Arriving here, prepare is complete, so we check the complete status.
	switch complete {
	case MachineSeriesUpgradeNotStarted:
		return UpgradeSeriesPrepareComplete, nil
	case MachineSeriesUpgradeStarted:
		return UpgradeSeriesCompleteStarted, nil
	case MachineSeriesUpgradeComplete:
		return UpgradeSeriesComplete, nil
	}

	return "", errors.Errorf("unable to determine status from old combination: %q/%q", prepare, complete)
}

// The Statuses, at least for units, appy to both the "Prepare" and "Complete"
// phases of a managed series upgrade. This type can be used to distinguish
// between those phases when working with the state of an upgraded.
type UpgradeSeriesStatusType string

const (
	PrepareStatus  UpgradeSeriesStatusType = "Prepare"
	CompleteStatus UpgradeSeriesStatusType = "Complete"
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
	PrepareStarted   UnitSeriesUpgradeStatus = "Started"
	PrepareCompleted UnitSeriesUpgradeStatus = "Completed"
	UnitErrored      UnitSeriesUpgradeStatus = "Errored"
)

// Validates a string returning an UpgradeSeriesPrepareStatus, if valid, or an error.
func ValidateUnitSeriesUpgradeStatus(series string) (UnitSeriesUpgradeStatus, error) {
	unCheckedStatus := UnitSeriesUpgradeStatus(series)
	switch unCheckedStatus {
	case NotStarted, PrepareStarted, UnitErrored, PrepareCompleted:
		return unCheckedStatus, nil
	}

	return NotStarted, errors.Errorf("encountered invalid unit upgrade series status of %q", series)
}
