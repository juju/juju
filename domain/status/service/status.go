// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
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

// StatusHistoryReader reads status history records.
type StatusHistoryReader interface {
	// Walk walks the status history, calling the given function for each
	// status record. The function should return true to continue walking, or
	// false to stop.
	Walk(func(statushistory.HistoryRecord) (bool, error)) error
	// Close closes the status history reader.
	Close() error
}

// StatusHistoryReaderFunc is a function that returns a StatusHistoryReader.
type StatusHistoryReaderFunc func() (StatusHistoryReader, error)

// encodeK8sPodStatusType converts a core status to a db cloud container
// status id.
func encodeK8sPodStatusType(s corestatus.Status) (status.K8sPodStatusType, error) {
	switch s {
	case corestatus.Unset:
		return status.K8sPodStatusUnset, nil
	case corestatus.Waiting:
		return status.K8sPodStatusWaiting, nil
	case corestatus.Blocked:
		return status.K8sPodStatusBlocked, nil
	case corestatus.Running:
		return status.K8sPodStatusRunning, nil
	default:
		return -1, errors.Errorf("unknown cloud container status %q", s)
	}
}

// encodeRelationStatusType maps a core status to corresponding db relation
// status.
func encodeRelationStatusType(s corestatus.Status) (status.RelationStatusType, error) {
	switch s {
	case corestatus.Joining:
		return status.RelationStatusTypeJoining, nil
	case corestatus.Joined:
		return status.RelationStatusTypeJoined, nil
	case corestatus.Broken:
		return status.RelationStatusTypeBroken, nil
	case corestatus.Suspending:
		return status.RelationStatusTypeSuspending, nil
	case corestatus.Suspended:
		return status.RelationStatusTypeSuspended, nil
	case corestatus.Error:
		return status.RelationStatusTypeError, nil
	default:
		return -1, errors.Errorf("unknown relation status %q", s)
	}
}

// decodeRelationStatusType maps a db relation status to a corresponding
// core status.
func decodeRelationStatusType(s status.RelationStatusType) (corestatus.Status, error) {
	switch s {
	case status.RelationStatusTypeJoining:
		return corestatus.Joining, nil
	case status.RelationStatusTypeJoined:
		return corestatus.Joined, nil
	case status.RelationStatusTypeBroken:
		return corestatus.Broken, nil
	case status.RelationStatusTypeSuspending:
		return corestatus.Suspending, nil
	case status.RelationStatusTypeSuspended:
		return corestatus.Suspended, nil
	case status.RelationStatusTypeError:
		return corestatus.Error, nil
	default:
		return "", errors.Errorf("unknown relation status %q", s)
	}
}

// decodeK8sPodStatusType converts a db cloud container status id to a
// core status.
func decodeK8sPodStatusType(s status.K8sPodStatusType) (corestatus.Status, error) {
	switch s {
	case status.K8sPodStatusUnset:
		return corestatus.Unset, nil
	case status.K8sPodStatusWaiting:
		return corestatus.Waiting, nil
	case status.K8sPodStatusBlocked:
		return corestatus.Blocked, nil
	case status.K8sPodStatusRunning:
		return corestatus.Running, nil
	default:
		return "", errors.Errorf("unknown cloud container status %q", s)
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

// encodeK8sPodStatus converts a core status info to a db status info.
func encodeK8sPodStatus(s corestatus.StatusInfo) (status.StatusInfo[status.K8sPodStatusType], error) {
	encodedStatus, err := encodeK8sPodStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.K8sPodStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.K8sPodStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeK8sPodStatus converts a db status info to a core status info.
func decodeK8sPodStatus(s status.StatusInfo[status.K8sPodStatusType]) (corestatus.StatusInfo, error) {
	decodedStatus, err := decodeK8sPodStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeRelationStatus converts a core status info into a db relation status
// info.
func encodeRelationStatus(s corestatus.StatusInfo) (status.StatusInfo[status.RelationStatusType], error) {
	encodedStatus, err := encodeRelationStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.RelationStatusType]{}, err
	}

	return status.StatusInfo[status.RelationStatusType]{
		Status:  encodedStatus,
		Since:   s.Since,
		Message: s.Message,
	}, nil
}

// encodeUnitAgentStatus converts a core status info to a db status info.
func encodeUnitAgentStatus(s corestatus.StatusInfo) (status.StatusInfo[status.UnitAgentStatusType], error) {
	encodedStatus, err := encodeUnitAgentStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.UnitAgentStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.UnitAgentStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.UnitAgentStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeUnitAgentStatus converts a db status info to a core status info.
func decodeUnitAgentStatus(s status.StatusInfo[status.UnitAgentStatusType], present bool) (corestatus.StatusInfo, error) {
	// If the agent isn't present then we need to modify the status for the
	// agent.
	if !present {
		return corestatus.StatusInfo{
			Status:  corestatus.Lost,
			Message: "agent is not communicating with the server",
			Since:   s.Since,
		}, nil
	}

	decodedStatus, err := decodeUnitAgentStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeWorkloadStatus converts a core status info to a db status info.
func encodeWorkloadStatus(s corestatus.StatusInfo) (status.StatusInfo[status.WorkloadStatusType], error) {
	encodedStatus, err := encodeWorkloadStatusType(s.Status)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return status.StatusInfo[status.WorkloadStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return status.StatusInfo[status.WorkloadStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeUnitWorkloadStatus converts a db status info to a core status info.
func decodeUnitWorkloadStatus(s status.StatusInfo[status.WorkloadStatusType], present bool) (corestatus.StatusInfo, error) {
	// If the workload isn't present then we need to modify the status for the
	// workload.
	if !present && !(s.Status == status.WorkloadStatusError ||
		s.Status == status.WorkloadStatusTerminated) {
		return corestatus.StatusInfo{
			Status:  corestatus.Unknown,
			Message: "agent lost, see `juju debug-logs` or `juju show-status-log` for more information",
			Since:   s.Since,
		}, nil
	}

	decodedStatus, err := decodeWorkloadStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

func decodeUnitWorkloadAgentStatus(s status.UnitWorkloadAgentStatus) (bool, corestatus.StatusInfo, corestatus.StatusInfo, error) {
	decodedAgentStatus, err := decodeUnitAgentStatusType(s.AgentStatus.Status)
	if err != nil {
		return false, corestatus.StatusInfo{}, corestatus.StatusInfo{}, err
	}
	decodedWorkloadStatus, err := decodeWorkloadStatusType(s.WorkloadStatus.Status)
	if err != nil {
		return false, corestatus.StatusInfo{}, corestatus.StatusInfo{}, err
	}

	var agentData map[string]interface{}
	if len(s.AgentStatus.Data) > 0 {
		if err := json.Unmarshal(s.AgentStatus.Data, &agentData); err != nil {
			return false, corestatus.StatusInfo{}, corestatus.StatusInfo{}, errors.Errorf("unmarshalling agent status data: %w", err)
		}
	}

	var workloadData map[string]interface{}
	if len(s.WorkloadStatus.Data) > 0 {
		if err := json.Unmarshal(s.WorkloadStatus.Data, &workloadData); err != nil {
			return false, corestatus.StatusInfo{}, corestatus.StatusInfo{}, errors.Errorf("unmarshalling workload status data: %w", err)
		}
	}

	return s.Present,
		corestatus.StatusInfo{
			Status:  decodedAgentStatus,
			Message: s.AgentStatus.Message,
			Data:    agentData,
			Since:   s.AgentStatus.Since,
		},
		corestatus.StatusInfo{
			Status:  decodedWorkloadStatus,
			Message: s.WorkloadStatus.Message,
			Data:    workloadData,
			Since:   s.WorkloadStatus.Since,
		}, nil
}

func decodeApplicationStatus(s status.StatusInfo[status.WorkloadStatusType]) (corestatus.StatusInfo, error) {
	decodedStatus, err := decodeWorkloadStatusType(s.Status)
	if err != nil {
		return corestatus.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return corestatus.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

func decodeUnitDisplayAndAgentStatus(
	fullUnitStatus status.FullUnitStatus,
) (corestatus.StatusInfo, corestatus.StatusInfo, error) {
	// If the unit agent is allocating, then it won't be present in the model.
	// In this case, we'll falsify the agent presence status.
	if fullUnitStatus.AgentStatus.Status == status.UnitAgentStatusAllocating {
		fullUnitStatus.Present = true
	}

	// If the agent is in an error state, we should set the workload status to be
	// in error state instead. Copy the data and message over from the agent to the
	// workload. The current 3.x system also does this, so we're attempting to
	// maintain the same behaviour. This can be disingenuous if there is a legitimate
	// agent error and the workload is fine, but we're trying to maintain compatibility.
	if fullUnitStatus.AgentStatus.Status == status.UnitAgentStatusError {
		var data map[string]interface{}
		if len(fullUnitStatus.AgentStatus.Data) > 0 {
			if err := json.Unmarshal(fullUnitStatus.AgentStatus.Data, &data); err != nil {
				return corestatus.StatusInfo{}, corestatus.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
			}
		}
		return corestatus.StatusInfo{
				Status: corestatus.Idle,
				Since:  fullUnitStatus.AgentStatus.Since,
			}, corestatus.StatusInfo{
				Status:  corestatus.Error,
				Since:   fullUnitStatus.WorkloadStatus.Since,
				Data:    data,
				Message: fullUnitStatus.AgentStatus.Message,
			}, nil
	}

	agentStatus, err := decodeUnitAgentStatus(fullUnitStatus.AgentStatus, fullUnitStatus.Present)
	if err != nil {
		return corestatus.StatusInfo{}, corestatus.StatusInfo{}, errors.Capture(err)
	}

	workloadStatus, err := selectWorkloadOrK8sPodStatus(fullUnitStatus.WorkloadStatus, fullUnitStatus.K8sPodStatus, fullUnitStatus.Present)
	if err != nil {
		return corestatus.StatusInfo{}, corestatus.StatusInfo{}, errors.Capture(err)
	}
	return agentStatus, workloadStatus, nil
}

func decodeUnitWorkloadStatuses(statuses status.UnitWorkloadStatuses) (map[unit.Name]corestatus.StatusInfo, error) {
	ret := make(map[unit.Name]corestatus.StatusInfo, len(statuses))
	for unitName, status := range statuses {
		info, err := decodeUnitWorkloadStatus(status.StatusInfo, status.Present)
		if err != nil {
			return nil, errors.Capture(err)
		}
		ret[unitName] = info
	}
	return ret, nil
}

func decodeUnitAgentStatusesWithoutPresence(statuses status.UnitAgentStatuses) (map[unit.Name]corestatus.StatusInfo, error) {
	ret := make(map[unit.Name]corestatus.StatusInfo, len(statuses))
	for unitName, status := range statuses {
		info, err := decodeUnitAgentStatus(status, true)
		if err != nil {
			return nil, errors.Capture(err)
		}
		ret[unitName] = info
	}
	return ret, nil
}

// selectWorkloadOrK8sPodStatus determines which of the two statuses to use
// when displaying unit status. It is used in CAAS models where the status of
// the unit could be overridden by the status of the container.
func selectWorkloadOrK8sPodStatus(
	workloadStatus status.StatusInfo[status.WorkloadStatusType],
	containerStatus status.StatusInfo[status.K8sPodStatusType],
	present bool,
) (corestatus.StatusInfo, error) {
	// container status is not set. This means that the unit is either a non-CAAS
	// unit or the container status has not been updated yet. Either way, we
	// should use the workload status.
	if containerStatus.Status == status.K8sPodStatusUnset {
		return decodeUnitWorkloadStatus(workloadStatus, present)
	}

	// statuses terminated, blocked and maintenance are statuses informed by the
	// charm, so these status always takes precedence.
	if workloadStatus.Status == status.WorkloadStatusTerminated ||
		workloadStatus.Status == status.WorkloadStatusBlocked ||
		workloadStatus.Status == status.WorkloadStatusMaintenance {
		return decodeUnitWorkloadStatus(workloadStatus, present)
	}

	// NOTE: We now know implicitly that the workload status is either active,
	// waiting or unknown.
	if containerStatus.Status == status.K8sPodStatusBlocked {
		return decodeK8sPodStatus(containerStatus)
	}

	if containerStatus.Status == status.K8sPodStatusWaiting {
		if workloadStatus.Status == status.WorkloadStatusActive {
			return decodeK8sPodStatus(containerStatus)
		}
	}

	if containerStatus.Status == status.K8sPodStatusRunning {
		if workloadStatus.Status == status.WorkloadStatusWaiting {
			return decodeK8sPodStatus(containerStatus)
		}
	}

	return decodeUnitWorkloadStatus(workloadStatus, present)
}

// statusSeverities holds status values with a severity measure.
// Status values with higher severity are used in preference to others.
var statusSeverities = map[corestatus.Status]int{
	corestatus.Error:       100,
	corestatus.Blocked:     90,
	corestatus.Maintenance: 80, // Maintenance (us busy) is higher than Waiting (someone else busy)
	corestatus.Waiting:     70,
	corestatus.Active:      60,
	corestatus.Terminated:  50,
	corestatus.Unknown:     40,
}

// applicationDisplayStatusFromUnits returns the status to display for an status
// based on both the workload and container statuses of its units.
func applicationDisplayStatusFromUnits(
	fullUnitStatuses status.FullUnitStatuses,
) (corestatus.StatusInfo, error) {
	results := make([]corestatus.StatusInfo, 0, len(fullUnitStatuses))

	for _, fullStatus := range fullUnitStatuses {
		_, displayStatus, err := decodeUnitDisplayAndAgentStatus(fullStatus)
		if err != nil {
			return corestatus.StatusInfo{}, errors.Capture(err)
		}
		results = append(results, displayStatus)
	}

	// By providing an unknown default, we get a reasonable answer
	// even if there are no units.
	result := corestatus.StatusInfo{
		Status: corestatus.Unknown,
	}
	for _, s := range results {
		if statusSeverities[s.Status] > statusSeverities[result.Status] {
			result = s
		}
	}
	return result, nil
}
