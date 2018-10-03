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
	if _, ok := UpgradeSeriesStatusOrder[status]; !ok {
		return UpgradeSeriesNotStarted, errors.NotValidf("upgrade series status of %q is", status)
	}
	return status, nil
}

// CompareUpgradeSeriesStatus compares two upgrade series statuses and returns and integer; if the first
// argument equals the second then 0 is returned; if the second is greater -1 is
// returned; 1 is returned otherwise.
func CompareUpgradeSeriesStatus(status1 UpgradeSeriesStatus, status2 UpgradeSeriesStatus) (int, error) {
	var err error
	st1, err := ValidateUnitSeriesUpgradeStatus(status1)
	st2, err := ValidateUnitSeriesUpgradeStatus(status2)
	if err != nil {
		return 0, err
	}

	if UpgradeSeriesStatusOrder[st1] == UpgradeSeriesStatusOrder[st2] {
		return 0, nil
	}
	if UpgradeSeriesStatusOrder[st1] < UpgradeSeriesStatusOrder[st2] {
		return -1, nil
	}
	return 1, nil
}
