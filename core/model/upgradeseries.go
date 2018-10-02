// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

// UpgradeSeriesStatus is the current status of a series upgrade for units
type UpgradeSeriesStatus string

var UpgradeSeriesStatusOrder map[UpgradeSeriesStatus]int = map[UpgradeSeriesStatus]int{
	UpgradeSeriesNotStarted:       0,
	UpgradeSeriesPrepareStarted:   1,
	UpgradeSeriesPrepareRunning:   2,
	UpgradeSeriesPrepareMachine:   3,
	UpgradeSeriesPrepareCompleted: 4,
	UpgradeSeriesCompleteStarted:  5,
	UpgradeSeriesCompleteRunning:  6,
	UpgradeSeriesCompleted:        7,
	UpgradeSeriesError:            8,
}

const (
	UpgradeSeriesNotStarted       UpgradeSeriesStatus = "not started"
	UpgradeSeriesPrepareStarted   UpgradeSeriesStatus = "prepare started"
	UpgradeSeriesPrepareRunning   UpgradeSeriesStatus = "prepare running"
	UpgradeSeriesPrepareMachine   UpgradeSeriesStatus = "prepare machine"
	UpgradeSeriesPrepareCompleted UpgradeSeriesStatus = "prepare completed"
	UpgradeSeriesCompleteStarted  UpgradeSeriesStatus = "complete started"
	UpgradeSeriesCompleteRunning  UpgradeSeriesStatus = "complete running"
	UpgradeSeriesCompleted        UpgradeSeriesStatus = "completed"
	UpgradeSeriesError            UpgradeSeriesStatus = "error"
)

// ValidateUnitSeriesUpgradeStatus validates a the input status as valid for a
// unit, returning the valid status or an error.
func ValidateUnitSeriesUpgradeStatus(status UpgradeSeriesStatus) (UpgradeSeriesStatus, error) {
	switch status {
	case UpgradeSeriesNotStarted, UpgradeSeriesPrepareStarted, UpgradeSeriesPrepareCompleted,
		UpgradeSeriesCompleteStarted, UpgradeSeriesCompleted, UpgradeSeriesError,
		UpgradeSeriesPrepareRunning, UpgradeSeriesCompleteRunning:
		return status, nil
	}
	return UpgradeSeriesNotStarted, errors.NotValidf("unit upgrade series status of %q", status)
}

// CompareUpgradeSeriesStatus compares two upgrade series statuses and returns and integer; if the first
// argument equals the second then 0 is returned; if the second is greater -1 is
// returned; 1 is returned otherwise.
func CompareUpgradeSeriesStatus(status1 UpgradeSeriesStatus, status2 UpgradeSeriesStatus) int {
	if UpgradeSeriesStatusOrder[status1] == UpgradeSeriesStatusOrder[status2] {
		return 0
	}
	if UpgradeSeriesStatusOrder[status1] < UpgradeSeriesStatusOrder[status2] {
		return -1
	}
	return 1
}
