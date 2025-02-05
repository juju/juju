package service

import (
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application"
)

// MarshallCloudContainerStatus converts a core status to a db cloud container status id.
func MarshallCloudContainerStatus(s status.Status) application.CloudContainerStatusType {
	switch s {
	case status.Waiting:
		return application.CloudContainerStatusWaiting
	case status.Blocked:
		return application.CloudContainerStatusBlocked
	case status.Running:
		return application.CloudContainerStatusRunning
	}
	return application.CloudContainerStatusWaiting
}

// MarshallUnitAgentStatus converts a core status to a db cunit agent status id.
func MarshallUnitAgentStatus(s status.Status) application.UnitAgentStatusType {
	switch s {
	case status.Allocating:
		return application.UnitAgentStatusAllocating
	case status.Executing:
		return application.UnitAgentStatusExecuting
	case status.Idle:
		return application.UnitAgentStatusIdle
	case status.Error:
		return application.UnitAgentStatusError
	case status.Failed:
		return application.UnitAgentStatusFailed
	case status.Lost:
		return application.UnitAgentStatusLost
	case status.Rebooting:
		return application.UnitAgentStatusRebooting
	}
	return application.UnitAgentStatusAllocating
}

// MarshallUnitWorkloadStatus converts a core status to a db unit workload status id.
func MarshallUnitWorkloadStatus(s status.Status) application.UnitWorkloadStatusType {
	switch s {
	case status.Unset:
		return application.UnitWorkloadStatusUnset
	case status.Unknown:
		return application.UnitWorkloadStatusUnknown
	case status.Maintenance:
		return application.UnitWorkloadStatusMaintenance
	case status.Waiting:
		return application.UnitWorkloadStatusWaiting
	case status.Blocked:
		return application.UnitWorkloadStatusBlocked
	case status.Active:
		return application.UnitWorkloadStatusActive
	case status.Terminated:
		return application.UnitWorkloadStatusTerminated
	}
	return application.UnitWorkloadStatusUnset
}
