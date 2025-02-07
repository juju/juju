// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

// decodeUnitWorkloadStatusType converts a db unit workload status id to a core.
// Implicitly validates the status type.
func decodeUnitWorkloadStatusType(s application.UnitWorkloadStatusType) (status.Status, error) {
	switch s {
	case application.UnitWorkloadStatusUnset:
		return status.Unset, nil
	case application.UnitWorkloadStatusUnknown:
		return status.Unknown, nil
	case application.UnitWorkloadStatusMaintenance:
		return status.Maintenance, nil
	case application.UnitWorkloadStatusWaiting:
		return status.Waiting, nil
	case application.UnitWorkloadStatusBlocked:
		return status.Blocked, nil
	case application.UnitWorkloadStatusActive:
		return status.Active, nil
	case application.UnitWorkloadStatusTerminated:
		return status.Terminated, nil
	default:
		return "", errors.Errorf("unknown workload status %q", s)
	}
}

// encodeCloudContainerStatus converts a core status info to a db status info.
func encodeCloudContainerStatus(s *status.StatusInfo) (*application.StatusInfo[application.CloudContainerStatusType], error) {
	if s == nil {
		return nil, nil
	}

	encodedStatus, err := encodeCloudContainerStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return nil, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  encodedStatus,
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

	encodedStatus, err := encodeUnitAgentStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return nil, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return &application.StatusInfo[application.UnitAgentStatusType]{
		Status:  encodedStatus,
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

	encodedStatus, err := encodeUnitWorkloadStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return nil, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return &application.StatusInfo[application.UnitWorkloadStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeCloudContainerStatus converts a db status info to a core status info.
// Implicitly validates the status.
func decodeUnitWorkloadStatus(s *application.StatusInfo[application.UnitWorkloadStatusType]) (*status.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	decodedStatus, err := decodeUnitWorkloadStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	data := make(map[string]interface{})
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return nil, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return &status.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil

}
