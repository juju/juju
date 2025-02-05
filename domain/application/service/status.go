package service

import (
	"encoding/json"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

// encodeCloudContainerStatusType converts a core status to a db cloud container
// status id.
func encodeCloudContainerStatusType(s status.Status) (application.CloudContainerStatusType, error) {
	switch s {
	case status.Waiting:
		return application.CloudContainerStatusWaiting, nil
	case status.Blocked:
		return application.CloudContainerStatusBlocked, nil
	case status.Running:
		return application.CloudContainerStatusRunning, nil
	default:
		return -1, errors.Errorf("unknown cloud container status %q", s)
	}
}

// encodeUnitAgentStatusType converts a core status to a db unit agent status id.
func encodeUnitAgentStatusType(s status.Status) (application.UnitAgentStatusType, error) {
	switch s {
	case status.Allocating:
		return application.UnitAgentStatusAllocating, nil
	case status.Executing:
		return application.UnitAgentStatusExecuting, nil
	case status.Idle:
		return application.UnitAgentStatusIdle, nil
	case status.Error:
		return application.UnitAgentStatusError, nil
	case status.Failed:
		return application.UnitAgentStatusFailed, nil
	case status.Lost:
		return application.UnitAgentStatusLost, nil
	case status.Rebooting:
		return application.UnitAgentStatusRebooting, nil
	default:
		return -1, errors.Errorf("unknown agent status %q", s)
	}
}

// encodeUnitWorkloadStatusType converts a core status to a db unit workload
// status id.
func encodeUnitWorkloadStatusType(s status.Status) (application.UnitWorkloadStatusType, error) {
	switch s {
	case status.Unset:
		return application.UnitWorkloadStatusUnset, nil
	case status.Unknown:
		return application.UnitWorkloadStatusUnknown, nil
	case status.Maintenance:
		return application.UnitWorkloadStatusMaintenance, nil
	case status.Waiting:
		return application.UnitWorkloadStatusWaiting, nil
	case status.Blocked:
		return application.UnitWorkloadStatusBlocked, nil
	case status.Active:
		return application.UnitWorkloadStatusActive, nil
	case status.Terminated:
		return application.UnitWorkloadStatusTerminated, nil
	default:
		return -1, errors.Errorf("unknown workload status %q", s)
	}
}

// encodeCloudContainerStatus converts a core status info to a db status info.
func encodeCloudContainerStatus(s *status.StatusInfo) (*application.StatusInfo[application.CloudContainerStatusType], error) {
	if s == nil {
		return nil, nil
	}

	status, err := encodeCloudContainerStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(s.Data)
	if err != nil {
		return nil, errors.Errorf("marshalling status data: %w", err)
	}

	return &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// encodeUnitAgentStatus converts a core status info to a db status info.
func encodeUnitAgentStatus(s *status.StatusInfo) (*application.StatusInfo[application.UnitAgentStatusType], error) {
	if s == nil {
		return nil, nil
	}

	status, err := encodeUnitAgentStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(s.Data)
	if err != nil {
		return nil, errors.Errorf("marshalling status data: %w", err)
	}

	return &application.StatusInfo[application.UnitAgentStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// encodeUnitWorkloadStatus converts a core status info to a db status info.
func encodeUnitWorkloadStatus(s *status.StatusInfo) (*application.StatusInfo[application.UnitWorkloadStatusType], error) {
	if s == nil {
		return nil, nil
	}

	status, err := encodeUnitWorkloadStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(s.Data)
	if err != nil {
		return nil, errors.Errorf("marshalling status data: %w", err)
	}

	return &application.StatusInfo[application.UnitWorkloadStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}
