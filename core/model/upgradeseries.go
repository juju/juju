// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

// UpgradeSeriesStatus is a status that a machine or unit can be in during the
// execution of an OS series upgrade on its host machine.
type UpgradeSeriesStatus int

// String returns an indicative description of the status.
func (s UpgradeSeriesStatus) String() string {
	return upgradeSeriesStatusNames[s]
}

// Enumerating the status values allows us to represent relative progression
// though the upgrade-series workflow.
const (
	UpgradeSeriesNotStarted UpgradeSeriesStatus = iota
	UpgradeSeriesPrepareStarted
	UpgradeSeriesPreparePending
	UpgradeSeriesPrepareComplete
	UpgradeSeriesCompleteStarted
	UpgradeSeriesCompletePending
	UpgradeSeriesComplete
	UpgradeSeriesError
)

var upgradeSeriesStatusNames = map[UpgradeSeriesStatus]string{
	UpgradeSeriesNotStarted:      "not started",
	UpgradeSeriesPrepareStarted:  "prepare started",
	UpgradeSeriesPreparePending:  "prepare pending",
	UpgradeSeriesPrepareComplete: "prepare complete",
	UpgradeSeriesCompleteStarted: "complete started",
	UpgradeSeriesCompletePending: "complete pending",
	UpgradeSeriesComplete:        "complete",
	UpgradeSeriesError:           "error",
}

// TODO (manadart 2018-08-07) Everything below here to be discarded.

// ToOldStatus is a shim between the new and old status representations.
// It exists to bootstrap that transition. Unit tests omitted by design.
func (s UpgradeSeriesStatus) ToOldStatus() (UpgradeSeriesStatusType, MachineSeriesUpgradeStatus) {
	switch s {
	case UpgradeSeriesNotStarted:
		return PrepareStatus, MachineSeriesUpgradeNotStarted
	case UpgradeSeriesPrepareStarted:
		return PrepareStatus, MachineSeriesUpgradeStarted
	case UpgradeSeriesPreparePending:
		return PrepareStatus, MachineSeriesUpgradeAgentsStopped
	case UpgradeSeriesPrepareComplete:
		return PrepareStatus, MachineSeriesUpgradeComplete
	case UpgradeSeriesCompleteStarted:
		return CompleteStatus, MachineSeriesUpgradeStarted
	case UpgradeSeriesCompletePending:
		return CompleteStatus, MachineSeriesUpgradeUnitsRunning
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
		return UpgradeSeriesCompleteStarted, nil
	case MachineSeriesUpgradeAgentsStopped:
		return UpgradeSeriesPreparePending, nil
	}

	// Arriving here, prepare is complete, so we check the complete status.
	switch complete {
	case MachineSeriesUpgradeNotStarted:
		return UpgradeSeriesPrepareComplete, nil
	case MachineSeriesUpgradeStarted:
		return UpgradeSeriesCompleteStarted, nil
	case MachineSeriesUpgradeUnitsRunning:
		return UpgradeSeriesCompletePending, nil
	case MachineSeriesUpgradeComplete:
		return UpgradeSeriesComplete, nil
	}

	return -1, errors.Errorf("unable to determine status from old combination: %q/%q", prepare, complete)
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
	MachineSeriesUpgradeUnitsRunning  MachineSeriesUpgradeStatus = "UnitsRunning"
	MachineSeriesUpgradeJujuComplete  MachineSeriesUpgradeStatus = "JujuComplete"
	MachineSeriesUpgradeAgentsStopped MachineSeriesUpgradeStatus = "AgentsStopped"
	MachineSeriesUpgradeComplete      MachineSeriesUpgradeStatus = "Complete"
)

//UnitSeriesUpgradeStatus is the current status of a series upgrade for units
type UnitSeriesUpgradeStatus string

const (
	UnitNotStarted UnitSeriesUpgradeStatus = "NotStarted"
	UnitStarted    UnitSeriesUpgradeStatus = "Started"
	UnitErrored    UnitSeriesUpgradeStatus = "Errored"
	UnitCompleted  UnitSeriesUpgradeStatus = "Completed"
)

// Validates a string returning an UpgradeSeriesPrepareStatus, if valid, or an error.
func ValidateUnitSeriesUpgradeStatus(series string) (UnitSeriesUpgradeStatus, error) {
	unCheckedStatus := UnitSeriesUpgradeStatus(series)
	switch unCheckedStatus {
	case UnitNotStarted, UnitStarted, UnitErrored, UnitCompleted:
		return unCheckedStatus, nil
	}

	return UnitNotStarted, errors.Errorf("encountered invalid unit upgrade series status of %q", series)
}
