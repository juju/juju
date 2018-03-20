// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"
)

// GoalStateStatus keeps the status and timestamp of a unit.
type GoalStateStatus struct {
	Status string
	Since  *time.Time
}

// UnitsGoalState keeps the collection of units and their GoalStateStatus
type UnitsGoalState map[string]GoalStateStatus

// GoalState is responsible to organize the Units and Relations with a
// specific unit, and transmit this information from the api to the worker.
type GoalState struct {
	Units     UnitsGoalState
	Relations map[string]UnitsGoalState
}
