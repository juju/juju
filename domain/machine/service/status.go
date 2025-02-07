// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
)

// encodeMachineStatusType converts a core status to a db unit workload
// status id.
func encodeMachineStatusType(s status.Status) (machine.MachineStatusType, error) {
	switch s {
	case status.Started:
		return machine.MachineStatusStarted, nil
	case status.Stopped:
		return machine.MachineStatusStopped, nil
	case status.Error:
		return machine.MachineStatusError, nil
	case status.Pending:
		return machine.MachineStatusPending, nil
	case status.Down:
		return machine.MachineStatusDown, nil
	default:
		return -1, errors.Errorf("unknown machine status %q", s)
	}
}

// decodeMachineStatusType converts a db unit workload status id to a core
// status.
func decodeMachineStatusType(s machine.MachineStatusType) (status.Status, error) {
	switch s {
	case machine.MachineStatusStarted:
		return status.Started, nil
	case machine.MachineStatusStopped:
		return status.Stopped, nil
	case machine.MachineStatusError:
		return status.Error, nil
	case machine.MachineStatusPending:
		return status.Pending, nil
	case machine.MachineStatusDown:
		return status.Down, nil
	default:
		return status.Unset, errors.Errorf("unknown machine status %d", s)
	}
}

// encodeMachineStatus converts a core status info to a db status info.
func encodeMachineStatus(s status.StatusInfo) (machine.StatusInfo[machine.MachineStatusType], error) {
	status, err := encodeMachineStatusType(s.Status)
	if err != nil {
		return machine.StatusInfo[machine.MachineStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return machine.StatusInfo[machine.MachineStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return machine.StatusInfo[machine.MachineStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeMachineStatus converts a db status info into a core status info.
func decodeMachineStatus(s machine.StatusInfo[machine.MachineStatusType]) (status.StatusInfo, error) {
	statusType, err := decodeMachineStatusType(s.Status)
	if err != nil {
		return status.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return status.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return status.StatusInfo{
		Status:  statusType,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}

// encodeInstanceStatusType converts a core status to a db unit workload
// status id.
func encodeInstanceStatusType(s status.Status) (machine.InstanceStatusType, error) {
	switch s {
	case status.Unset:
		return machine.InstanceStatusUnset, nil
	case status.Running:
		return machine.InstanceStatusRunning, nil
	case status.Provisioning:
		return machine.InstanceStatusAllocating, nil
	case status.ProvisioningError:
		return machine.InstanceStatusProvisioningError, nil
	default:
		return -1, errors.Errorf("unknown machine status %q", s)
	}
}

// decodeInstanceStatusType converts a db unit workload status id to a core
// status.
func decodeInstanceStatusType(s machine.InstanceStatusType) (status.Status, error) {
	switch s {
	case machine.InstanceStatusUnset:
		return status.Unset, nil
	case machine.InstanceStatusRunning:
		return status.Running, nil
	case machine.InstanceStatusAllocating:
		return status.Provisioning, nil
	case machine.InstanceStatusProvisioningError:
		return status.ProvisioningError, nil
	default:
		return status.Unset, errors.Errorf("unknown machine status %d", s)
	}
}

// encodeInstanceStatus converts a core status info to a db status info.
func encodeInstanceStatus(s status.StatusInfo) (machine.StatusInfo[machine.InstanceStatusType], error) {
	status, err := encodeInstanceStatusType(s.Status)
	if err != nil {
		return machine.StatusInfo[machine.InstanceStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return machine.StatusInfo[machine.InstanceStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return machine.StatusInfo[machine.InstanceStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeInstanceStatus converts a db status info into a core status info.
func decodeInstanceStatus(s machine.StatusInfo[machine.InstanceStatusType]) (status.StatusInfo, error) {
	statusType, err := decodeInstanceStatusType(s.Status)
	if err != nil {
		return status.StatusInfo{}, err
	}

	var data map[string]interface{}
	if len(s.Data) > 0 {
		if err := json.Unmarshal(s.Data, &data); err != nil {
			return status.StatusInfo{}, errors.Errorf("unmarshalling status data: %w", err)
		}
	}

	return status.StatusInfo{
		Status:  statusType,
		Message: s.Message,
		Data:    data,
		Since:   s.Since,
	}, nil
}
