// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, corestatus.StatusInfo) error
}

// encodeCloudContainerStatusType converts a core status to a db cloud container
// status id.
func encodeCloudContainerStatusType(s corestatus.Status) (status.CloudContainerStatusType, error) {
	switch s {
	case corestatus.Unset:
		return status.CloudContainerStatusUnset, nil
	case corestatus.Waiting:
		return status.CloudContainerStatusWaiting, nil
	case corestatus.Blocked:
		return status.CloudContainerStatusBlocked, nil
	case corestatus.Running:
		return status.CloudContainerStatusRunning, nil
	default:
		return -1, errors.Errorf("unknown cloud container status %q", s)
	}
}

// encodeUnitAgentStatusType converts a core status to a db unit agent status id.
func encodeUnitAgentStatusType(s corestatus.Status) (status.UnitAgentStatusType, error) {
	switch s {
	case corestatus.Allocating:
		return status.UnitAgentStatusAllocating, nil
	case corestatus.Executing:
		return status.UnitAgentStatusExecuting, nil
	case corestatus.Idle:
		return status.UnitAgentStatusIdle, nil
	case corestatus.Error:
		return status.UnitAgentStatusError, nil
	case corestatus.Failed:
		return status.UnitAgentStatusFailed, nil
	case corestatus.Lost:
		return status.UnitAgentStatusLost, nil
	case corestatus.Rebooting:
		return status.UnitAgentStatusRebooting, nil
	default:
		return -1, errors.Errorf("unknown agent status %q", s)
	}
}

// decodeUnitAgentStatusType converts a db unit agent status id to a core
// status.
func decodeUnitAgentStatusType(s status.UnitAgentStatusType) (corestatus.Status, error) {
	switch s {
	case status.UnitAgentStatusAllocating:
		return corestatus.Allocating, nil
	case status.UnitAgentStatusExecuting:
		return corestatus.Executing, nil
	case status.UnitAgentStatusIdle:
		return corestatus.Idle, nil
	case status.UnitAgentStatusError:
		return corestatus.Error, nil
	case status.UnitAgentStatusFailed:
		return corestatus.Failed, nil
	case status.UnitAgentStatusLost:
		return corestatus.Lost, nil
	case status.UnitAgentStatusRebooting:
		return corestatus.Rebooting, nil
	default:
		return "", errors.Errorf("unknown agent status %q", s)
	}
}

// encodeWorkloadStatusType converts a core status to a db unit workload and
// application status id.
func encodeWorkloadStatusType(s corestatus.Status) (status.WorkloadStatusType, error) {
	switch s {
	case corestatus.Unset:
		return status.WorkloadStatusUnset, nil
	case corestatus.Unknown:
		return status.WorkloadStatusUnknown, nil
	case corestatus.Maintenance:
		return status.WorkloadStatusMaintenance, nil
	case corestatus.Waiting:
		return status.WorkloadStatusWaiting, nil
	case corestatus.Blocked:
		return status.WorkloadStatusBlocked, nil
	case corestatus.Active:
		return status.WorkloadStatusActive, nil
	case corestatus.Terminated:
		return status.WorkloadStatusTerminated, nil
	case corestatus.Error:
		return status.WorkloadStatusError, nil
	default:
		return -1, errors.Errorf("unknown workload status %q", s)
	}
}

// decodeWorkloadStatusType converts a db unit workload status id to a core.
// Implicitly validates the status type.
func decodeWorkloadStatusType(s status.WorkloadStatusType) (corestatus.Status, error) {
	switch s {
	case status.WorkloadStatusUnset:
		return corestatus.Unset, nil
	case status.WorkloadStatusUnknown:
		return corestatus.Unknown, nil
	case status.WorkloadStatusMaintenance:
		return corestatus.Maintenance, nil
	case status.WorkloadStatusWaiting:
		return corestatus.Waiting, nil
	case status.WorkloadStatusBlocked:
		return corestatus.Blocked, nil
	case status.WorkloadStatusActive:
		return corestatus.Active, nil
	case status.WorkloadStatusTerminated:
		return corestatus.Terminated, nil
	case status.WorkloadStatusError:
		return corestatus.Error, nil
	default:
		return "", errors.Errorf("unknown workload status %q", s)
	}
}

// encodeCloudContainerStatus converts a core status info to a db status info.
//
// TODO(jack-w-shaw): This function should be imported from the status domain instead
// of implemented here.
func encodeCloudContainerStatus(s *corestatus.StatusInfo) (*status.StatusInfo[status.CloudContainerStatusType], error) {
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

	return &status.StatusInfo[status.CloudContainerStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// encodeUnitAgentStatus converts a core status info to a db status info.
//
// TODO(jack-w-shaw): This function should be imported from the status domain instead
// of implemented here.
func encodeUnitAgentStatus(s *corestatus.StatusInfo) (*status.StatusInfo[status.UnitAgentStatusType], error) {
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

	return &status.StatusInfo[status.UnitAgentStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeUnitAgentStatus converts a db status info to a core status info.
//
// TODO(jack-w-shaw): This function should be imported from the status domain instead
// of implemented here.
func decodeUnitAgentStatus(s *status.UnitStatusInfo[status.UnitAgentStatusType]) (*corestatus.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	// If the agent isn't present then we need to modify the status for the
	// agent.
	if !s.Present {
		return &corestatus.StatusInfo{
			Status:  corestatus.Lost,
			Message: "agent is not communicating with the server",
			Since:   s.Since,
		}, nil
	}

	// If the agent is in an error state, the workload status should also be in
	// error state as well. The current 3.x system also does this, so we're
	// attempting to maintain the same behaviour. This can be disingenuous if
	// there is a legitimate agent error and the workload is fine, but we're
	// trying to maintain compatibility.
	if s.Status == status.UnitAgentStatusError {
		return &corestatus.StatusInfo{
			Status: corestatus.Idle,
			Since:  s.Since,
		}, nil
	}

	decodedStatus, err := decodeUnitAgentStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return nil, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return &corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeWorkloadStatus converts a core status info to a db status info.
//
// TODO(jack-w-shaw): This function should be imported from the status domain instead
// of implemented here.
func encodeWorkloadStatus(s *corestatus.StatusInfo) (*status.StatusInfo[status.WorkloadStatusType], error) {
	if s == nil {
		return nil, nil
	}

	encodedStatus, err := encodeWorkloadStatusType(s.Status)
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

	return &status.StatusInfo[status.WorkloadStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeUnitWorkloadStatus converts a db status info to a core status info.
//
// TODO(jack-w-shaw): This function should be imported from the status domain instead
// of implemented here.
func decodeUnitWorkloadStatus(s *status.UnitStatusInfo[status.WorkloadStatusType]) (*corestatus.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	// If the workload isn't present then we need to modify the status for the
	// workload.
	if !s.Present && !(s.Status == status.WorkloadStatusError ||
		s.Status == status.WorkloadStatusTerminated) {
		return &corestatus.StatusInfo{
			Status:  corestatus.Unknown,
			Message: "agent lost, see `juju debug-logs` or `juju show-status-log` for more information",
			Since:   s.Since,
		}, nil
	}

	decodedStatus, err := decodeWorkloadStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return nil, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return &corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}
