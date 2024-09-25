// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	corestatus "github.com/juju/juju/core/status"
)

// CloudContainerStatusType represents the status of a cloud container
// as recorded in the cloud_container_status_value lookup table.
type CloudContainerStatusType int

const (
	CloudContainerStatusWaiting CloudContainerStatusType = iota
	CloudContainerStatusBlocked
	CloudContainerStatusRunning
)

// MarshallCloudContainerStatus converts a core status to a db cloud container status id.
func MarshallCloudContainerStatus(status corestatus.Status) CloudContainerStatusType {
	switch status {
	case corestatus.Waiting:
		return CloudContainerStatusWaiting
	case corestatus.Blocked:
		return CloudContainerStatusBlocked
	case corestatus.Running:
		return CloudContainerStatusRunning
	}
	return CloudContainerStatusWaiting
}

// UnitAgentStatusType represents the status of a unit agent
// as recorded in the unit_agent_status_value lookup table.
type UnitAgentStatusType int

const (
	UnitAgentStatusAllocating UnitAgentStatusType = iota
	UnitAgentStatusExecuting
	UnitAgentStatusIdle
	UnitAgentStatusError
	UnitAgentStatusFailed
	UnitAgentStatusLost
	UnitAgentStatusRebooting
)

// MarshallUnitAgentStatus converts a core status to a db cunit agent status id.
func MarshallUnitAgentStatus(status corestatus.Status) UnitAgentStatusType {
	switch status {
	case corestatus.Allocating:
		return UnitAgentStatusAllocating
	case corestatus.Executing:
		return UnitAgentStatusExecuting
	case corestatus.Idle:
		return UnitAgentStatusIdle
	case corestatus.Error:
		return UnitAgentStatusError
	case corestatus.Failed:
		return UnitAgentStatusFailed
	case corestatus.Lost:
		return UnitAgentStatusLost
	case corestatus.Rebooting:
		return UnitAgentStatusRebooting
	}
	return UnitAgentStatusAllocating
}

// UnitWorkloadStatusType represents the status of a unit workload
// as recorded in the unit_workload_status_value lookup table.
type UnitWorkloadStatusType int

const (
	UnitWorkloadStatusUnset UnitWorkloadStatusType = iota
	UnitWorkloadStatusUnknown
	UnitWorkloadStatusMaintenance
	UnitWorkloadStatusWaiting
	UnitWorkloadStatusBlocked
	UnitWorkloadStatusActive
	UnitWorkloadStatusTerminated
)

// MarshallUnitWorkloadStatus converts a core status to a db unit workload status id.
func MarshallUnitWorkloadStatus(status corestatus.Status) UnitWorkloadStatusType {
	switch status {
	case corestatus.Unset:
		return UnitWorkloadStatusUnset
	case corestatus.Unknown:
		return UnitWorkloadStatusUnknown
	case corestatus.Maintenance:
		return UnitWorkloadStatusMaintenance
	case corestatus.Waiting:
		return UnitWorkloadStatusWaiting
	case corestatus.Blocked:
		return UnitWorkloadStatusBlocked
	case corestatus.Active:
		return UnitWorkloadStatusActive
	case corestatus.Terminated:
		return UnitWorkloadStatusTerminated
	}
	return UnitWorkloadStatusUnset
}
