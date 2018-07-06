// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

//MachineSeriesUpgradeStatus is the current status a machine series upgrade
type MachineSeriesUpgradeStatus string

const (
	NotStarted    MachineSeriesUpgradeStatus = "NotStarted"
	Started       MachineSeriesUpgradeStatus = "Started"
	UnitsRunning  MachineSeriesUpgradeStatus = "UnitsRunning"
	JujuComplete  MachineSeriesUpgradeStatus = "JujuComplete"
	AgentsStopped MachineSeriesUpgradeStatus = "AgentsStopped"
	Complete      MachineSeriesUpgradeStatus = "Complete"
)

//UnitSeriesUpgradeStatus is the current status of an upgrade
type UnitSeriesUpgradeStatus string

const (
	UnitNotStarted UnitSeriesUpgradeStatus = "UnitNotStarted"
	UnitStarted    UnitSeriesUpgradeStatus = "UnitStarted"
	UnitErrored    UnitSeriesUpgradeStatus = "UnitErrored"
	UnitCompleted  UnitSeriesUpgradeStatus = "UnitCompleted"
)
