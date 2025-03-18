// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"time"

	coreunit "github.com/juju/juju/core/unit"
)

// StatusID represents the status of an entity.
type StatusID interface {
	UnsetStatusType | CloudContainerStatusType | UnitAgentStatusType | WorkloadStatusType
}

// StatusInfo holds details about the status of an entity.
type StatusInfo[T StatusID] struct {
	Status  T
	Message string
	Data    []byte
	Since   *time.Time
}

// UnitStatusID represents the status of a unit.
type UnitStatusID interface {
	UnitAgentStatusType | WorkloadStatusType
}

// UnitAgentStatusInfo holds details about the status of a unit agent. This
// indicates if the unit agent is present and currently active in the model.
type UnitStatusInfo[T UnitStatusID] struct {
	StatusInfo[T]
	// Present is true if the unit agent logged into the API server.
	Present bool
}

// UnsetStatusType represents the status of an entity that has not been set.
type UnsetStatusType int

const (
	UnsetStatus UnsetStatusType = iota
)

// CloudContainerStatusType represents the status of a cloud container
// as recorded in the k8s_pod_status_value lookup table.
type CloudContainerStatusType int

const (
	CloudContainerStatusWaiting CloudContainerStatusType = iota
	CloudContainerStatusBlocked
	CloudContainerStatusRunning
)

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

// WorkloadStatusType represents the status of a unit workload or application
// as recorded in the workload_status_value lookup table.
type WorkloadStatusType int

const (
	WorkloadStatusUnset WorkloadStatusType = iota
	WorkloadStatusUnknown
	WorkloadStatusMaintenance
	WorkloadStatusWaiting
	WorkloadStatusBlocked
	WorkloadStatusActive
	WorkloadStatusTerminated
	WorkloadStatusError
)

// UnitWorkloadStatuses represents the workload statuses of a collection of units.
// The statuses are indexed by unit name.
type UnitWorkloadStatuses map[coreunit.Name]UnitStatusInfo[WorkloadStatusType]

// UnitCloudContainerStatuses represents the cloud container statuses of a collection
// of units. The statuses are indexed by unit name.
type UnitCloudContainerStatuses map[coreunit.Name]StatusInfo[CloudContainerStatusType]
