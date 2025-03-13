// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, status.StatusInfo) error
}

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

// decodeCloudContainerStatusType converts a db cloud container status id to a
// core status.
func decodeCloudContainerStatusType(s application.CloudContainerStatusType) (status.Status, error) {
	switch s {
	case application.CloudContainerStatusWaiting:
		return status.Waiting, nil
	case application.CloudContainerStatusBlocked:
		return status.Blocked, nil
	case application.CloudContainerStatusRunning:
		return status.Running, nil
	default:
		return "", errors.Errorf("unknown cloud container status %q", s)
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

// decodeUnitAgentStatusType converts a db unit agent status id to a core
// status.
func decodeUnitAgentStatusType(s application.UnitAgentStatusType) (status.Status, error) {
	switch s {
	case application.UnitAgentStatusAllocating:
		return status.Allocating, nil
	case application.UnitAgentStatusExecuting:
		return status.Executing, nil
	case application.UnitAgentStatusIdle:
		return status.Idle, nil
	case application.UnitAgentStatusError:
		return status.Error, nil
	case application.UnitAgentStatusFailed:
		return status.Failed, nil
	case application.UnitAgentStatusLost:
		return status.Lost, nil
	case application.UnitAgentStatusRebooting:
		return status.Rebooting, nil
	default:
		return "", errors.Errorf("unknown agent status %q", s)
	}
}

// encodeWorkloadStatusType converts a core status to a db unit workload and
// application status id.
func encodeWorkloadStatusType(s status.Status) (application.WorkloadStatusType, error) {
	switch s {
	case status.Unset:
		return application.WorkloadStatusUnset, nil
	case status.Unknown:
		return application.WorkloadStatusUnknown, nil
	case status.Maintenance:
		return application.WorkloadStatusMaintenance, nil
	case status.Waiting:
		return application.WorkloadStatusWaiting, nil
	case status.Blocked:
		return application.WorkloadStatusBlocked, nil
	case status.Active:
		return application.WorkloadStatusActive, nil
	case status.Terminated:
		return application.WorkloadStatusTerminated, nil
	case status.Error:
		return application.WorkloadStatusError, nil
	default:
		return -1, errors.Errorf("unknown workload status %q", s)
	}
}

// decodeWorkloadStatusType converts a db unit workload status id to a core.
// Implicitly validates the status type.
func decodeWorkloadStatusType(s application.WorkloadStatusType) (status.Status, error) {
	switch s {
	case application.WorkloadStatusUnset:
		return status.Unset, nil
	case application.WorkloadStatusUnknown:
		return status.Unknown, nil
	case application.WorkloadStatusMaintenance:
		return status.Maintenance, nil
	case application.WorkloadStatusWaiting:
		return status.Waiting, nil
	case application.WorkloadStatusBlocked:
		return status.Blocked, nil
	case application.WorkloadStatusActive:
		return status.Active, nil
	case application.WorkloadStatusTerminated:
		return status.Terminated, nil
	case application.WorkloadStatusError:
		return status.Error, nil
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

// decodeCloudContainerStatus converts a db status info to a core status info.
func decodeCloudContainerStatus(s *application.StatusInfo[application.CloudContainerStatusType]) (*status.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	decodedStatus, err := decodeCloudContainerStatusType(s.Status)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
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

// decodeUnitAgentStatus converts a db status info to a core status info.
func decodeUnitAgentStatus(s *application.UnitStatusInfo[application.UnitAgentStatusType]) (*status.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	// If the agent isn't present then we need to modify the status for the
	// agent.
	if !s.Present {
		return &status.StatusInfo{
			Status:  status.Lost,
			Message: "agent is not communicating with the server",
			Since:   s.Since,
		}, nil
	}

	// If the agent is in an error state, the workload status should also be in
	// error state as well. The current 3.x system also does this, so we're
	// attempting to maintain the same behaviour. This can be disingenuous if
	// there is a legitimate agent error and the workload is fine, but we're
	// trying to maintain compatibility.
	if s.Status == application.UnitAgentStatusError {
		return &status.StatusInfo{
			Status: status.Idle,
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

	return &status.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeWorkloadStatus converts a core status info to a db status info.
func encodeWorkloadStatus(s *status.StatusInfo) (*application.StatusInfo[application.WorkloadStatusType], error) {
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

	return &application.StatusInfo[application.WorkloadStatusType]{
		Status:  encodedStatus,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeUnitWorkloadStatus converts a db status info to a core status info.
func decodeUnitWorkloadStatus(s *application.UnitStatusInfo[application.WorkloadStatusType]) (*status.StatusInfo, error) {
	if s == nil {
		return nil, nil
	}

	// If the workload isn't present then we need to modify the status for the
	// workload.
	if !s.Present && !(s.Status == application.WorkloadStatusError ||
		s.Status == application.WorkloadStatusTerminated) {
		return &status.StatusInfo{
			Status:  status.Unknown,
			Message: fmt.Sprintf("agent lost, see `juju debug-logs` or `juju show-status-log` for more information"),
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

	return &status.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

func decodeApplicationStatus(s *application.StatusInfo[application.WorkloadStatusType]) (*status.StatusInfo, error) {
	if s == nil {
		return nil, nil
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

	return &status.StatusInfo{
		Status:  decodedStatus,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

func decodeUnitAgentWorkloadStatus(
	agent *application.UnitStatusInfo[application.UnitAgentStatusType],
	workload *application.UnitStatusInfo[application.WorkloadStatusType],
	containerStatus *application.StatusInfo[application.CloudContainerStatusType],
) (*status.StatusInfo, *status.StatusInfo, error) {
	// If the unit agent is allocating, then it won't be present in the model.
	// In this case, we'll falsify the agent presence status.
	if agent.Status == application.UnitAgentStatusAllocating {
		agent.Present = true
		workload.Present = true
	}

	agentStatus, err := decodeUnitAgentStatus(agent)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	workloadStatus, err := unitDisplayStatus(workload, containerStatus)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	return agentStatus, workloadStatus, nil
}

// reduceUnitWorkloadStatuses reduces a list of workload statuses to a single status.
// We do this by taking the highest priority status from the list.
func reduceUnitWorkloadStatuses(statuses []application.UnitStatusInfo[application.WorkloadStatusType]) (*status.StatusInfo, error) {
	// By providing an unknown default, we get a reasonable answer
	// even if there are no units.
	result := &status.StatusInfo{
		Status: status.Unknown,
	}
	for _, s := range statuses {
		decodedStatus, err := decodeUnitWorkloadStatus(&s)
		if err != nil {
			return nil, errors.Capture(err)
		}

		if statusSeverities[decodedStatus.Status] > statusSeverities[result.Status] {
			result = decodedStatus
		}
	}
	return result, nil
}

func decodeUnitWorkloadStatuses(statuses application.UnitWorkloadStatuses) (map[unit.Name]status.StatusInfo, error) {
	ret := make(map[unit.Name]status.StatusInfo, len(statuses))
	for unitName, status := range statuses {
		info, err := decodeUnitWorkloadStatus(&status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		ret[unitName] = *info
	}
	return ret, nil
}

// statusSeverities holds status values with a severity measure.
// Status values with higher severity are used in preference to others.
var statusSeverities = map[status.Status]int{
	status.Error:       100,
	status.Blocked:     90,
	status.Maintenance: 80, // Maintenance (us busy) is higher than Waiting (someone else busy)
	status.Waiting:     70,
	status.Active:      60,
	status.Terminated:  50,
	status.Unknown:     40,
}

// unitDisplayStatus determines which of the two statuses to use when displaying
// unit status. It is used in CAAS models where the status of the unit could be
// overridden by the status of the container.
func unitDisplayStatus(
	workloadStatus *application.UnitStatusInfo[application.WorkloadStatusType],
	containerStatus *application.StatusInfo[application.CloudContainerStatusType],
) (*status.StatusInfo, error) {
	// container status is not set. This means that the unit is either a non-CAAS
	// unit or the container status has not been updated yet. Either way, we
	// should use the workload status.
	if containerStatus == nil {
		return decodeUnitWorkloadStatus(workloadStatus)
	}

	// statuses terminated, blocked and maintenance are statuses informed by the
	// charm, so these status always takes precedence.
	if workloadStatus.Status == application.WorkloadStatusTerminated ||
		workloadStatus.Status == application.WorkloadStatusBlocked ||
		workloadStatus.Status == application.WorkloadStatusMaintenance {
		return decodeUnitWorkloadStatus(workloadStatus)
	}

	// NOTE: We now know implicitly that the workload status is either active,
	// waiting or unknown.

	if containerStatus.Status == application.CloudContainerStatusBlocked {
		return decodeCloudContainerStatus(containerStatus)
	}

	if containerStatus.Status == application.CloudContainerStatusWaiting {
		if workloadStatus.Status == application.WorkloadStatusActive {
			return decodeCloudContainerStatus(containerStatus)
		}
	}

	if containerStatus.Status == application.CloudContainerStatusRunning {
		if workloadStatus.Status == application.WorkloadStatusWaiting {
			return decodeCloudContainerStatus(containerStatus)
		}
	}

	return decodeUnitWorkloadStatus(workloadStatus)
}

// applicationDisplayStatusFromUnits returns the status to display for an application
// based on both the workload and container statuses of its units.
func applicationDisplayStatusFromUnits(
	workloadStatus application.UnitWorkloadStatuses,
	containerStatus application.UnitCloudContainerStatuses,
) (*status.StatusInfo, error) {
	results := make([]*status.StatusInfo, 0, len(workloadStatus))

	for unitUUID, workload := range workloadStatus {
		var unitStatus *status.StatusInfo

		container, ok := containerStatus[unitUUID]
		if !ok {
			var err error
			unitStatus, err = unitDisplayStatus(&workload, nil)
			if err != nil {
				return nil, errors.Capture(err)
			}
		} else {
			var err error
			unitStatus, err = unitDisplayStatus(&workload, &container)
			if err != nil {
				return nil, errors.Capture(err)
			}
		}

		results = append(results, unitStatus)
	}

	// By providing an unknown default, we get a reasonable answer
	// even if there are no units.
	result := &status.StatusInfo{
		Status: status.Unknown,
	}
	for _, s := range results {
		if statusSeverities[s.Status] > statusSeverities[result.Status] {
			result = s
		}
	}
	return result, nil
}
