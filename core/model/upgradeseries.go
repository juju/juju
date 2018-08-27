// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
)

// UpgradeSeriesStatus is the current status of a series upgrade for units
type UpgradeSeriesStatus string

const (
	UpgradeSeriesNotStarted       UpgradeSeriesStatus = "not started"
	UpgradeSeriesPrepareStarted   UpgradeSeriesStatus = "prepare started"
	UpgradeSeriesPrepareMachine   UpgradeSeriesStatus = "prepare machine"
	UpgradeSeriesPrepareCompleted UpgradeSeriesStatus = "prepare completed"
	UpgradeSeriesCompleteStarted  UpgradeSeriesStatus = "complete started"
	UpgradeSeriesCompleted        UpgradeSeriesStatus = "completed"
	UpgradeSeriesError            UpgradeSeriesStatus = "error"
)

// ValidateUnitSeriesUpgradeStatus validates a the input status as valid for a
// unit, returning the valid status or an error.
func ValidateUnitSeriesUpgradeStatus(status UpgradeSeriesStatus) (UpgradeSeriesStatus, error) {
	switch status {
	case UpgradeSeriesNotStarted, UpgradeSeriesPrepareStarted, UpgradeSeriesPrepareCompleted,
		UpgradeSeriesCompleteStarted, UpgradeSeriesCompleted, UpgradeSeriesError:
		return status, nil
	}
	return UpgradeSeriesNotStarted, errors.NotValidf("unit upgrade series status of %q", status)
}
