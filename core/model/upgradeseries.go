// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
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

// Validates a string returning an UpgradeSeriesStatus, if valid, or an error.
func ValidateUnitSeriesUpgradeStatus(series string) (UnitSeriesUpgradeStatus, error) {
	unitSeriesUpgradeStatuses := []UnitSeriesUpgradeStatus{UnitNotStarted, UnitStarted, UnitCompleted, UnitErrored}
	i, found := indexOf(UnitSeriesUpgradeStatus(series), unitSeriesUpgradeStatuses)
	if !found {
		return UnitNotStarted, errors.New("Invalid Unit Upgrade Series Status")
	}
	return unitSeriesUpgradeStatuses[i], nil
}

func indexOf(find UnitSeriesUpgradeStatus, in []UnitSeriesUpgradeStatus) (int, bool) {
	for ix, cur := range in {
		if cur == find {
			return ix, true
		}
	}
	return -1, false
}
