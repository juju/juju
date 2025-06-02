// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"

	"github.com/juju/juju/core/status"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// encodeMachineStatusType converts a core status to a db unit workload
// status id.
func encodeMachineStatusType(s status.Status) (domainstatus.MachineStatusType, error) {
	switch s {
	case status.Started:
		return domainstatus.MachineStatusStarted, nil
	case status.Stopped:
		return domainstatus.MachineStatusStopped, nil
	case status.Error:
		return domainstatus.MachineStatusError, nil
	case status.Pending:
		return domainstatus.MachineStatusPending, nil
	case status.Down:
		return domainstatus.MachineStatusDown, nil
	default:
		return -1, errors.Errorf("unknown machine status %q", s)
	}
}

// decodeMachineStatusType converts a db unit workload status id to a core
// status.
func decodeMachineStatusType(s domainstatus.MachineStatusType) (status.Status, error) {
	switch s {
	case domainstatus.MachineStatusStarted:
		return status.Started, nil
	case domainstatus.MachineStatusStopped:
		return status.Stopped, nil
	case domainstatus.MachineStatusError:
		return status.Error, nil
	case domainstatus.MachineStatusPending:
		return status.Pending, nil
	case domainstatus.MachineStatusDown:
		return status.Down, nil
	default:
		return status.Unset, errors.Errorf("unknown machine status %d", s)
	}
}

// encodeMachineStatus converts a core status info to a db status info.
func encodeMachineStatus(s status.StatusInfo) (domainstatus.StatusInfo[domainstatus.MachineStatusType], error) {
	status, err := encodeMachineStatusType(s.Status)
	if err != nil {
		return domainstatus.StatusInfo[domainstatus.MachineStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return domainstatus.StatusInfo[domainstatus.MachineStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return domainstatus.StatusInfo[domainstatus.MachineStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeMachineStatus converts a db status info into a core status info.
func decodeMachineStatus(s domainstatus.StatusInfo[domainstatus.MachineStatusType]) (status.StatusInfo, error) {
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
func encodeInstanceStatusType(s status.Status) (domainstatus.InstanceStatusType, error) {
	switch s {
	case status.Unset:
		return domainstatus.InstanceStatusUnset, nil
	case status.Running:
		return domainstatus.InstanceStatusRunning, nil
	case status.Provisioning:
		return domainstatus.InstanceStatusAllocating, nil
	case status.ProvisioningError:
		return domainstatus.InstanceStatusProvisioningError, nil
	default:
		return -1, errors.Errorf("unknown machine status %q", s)
	}
}

// decodeInstanceStatusType converts a db unit workload status id to a core
// status.
func decodeInstanceStatusType(s domainstatus.InstanceStatusType) (status.Status, error) {
	switch s {
	case domainstatus.InstanceStatusUnset:
		return status.Unset, nil
	case domainstatus.InstanceStatusRunning:
		return status.Running, nil
	case domainstatus.InstanceStatusAllocating:
		return status.Provisioning, nil
	case domainstatus.InstanceStatusProvisioningError:
		return status.ProvisioningError, nil
	default:
		return status.Unset, errors.Errorf("unknown machine status %d", s)
	}
}

// encodeInstanceStatus converts a core status info to a db status info.
func encodeInstanceStatus(s status.StatusInfo) (domainstatus.StatusInfo[domainstatus.InstanceStatusType], error) {
	status, err := encodeInstanceStatusType(s.Status)
	if err != nil {
		return domainstatus.StatusInfo[domainstatus.InstanceStatusType]{}, err
	}

	var bytes []byte
	if len(s.Data) > 0 {
		var err error
		bytes, err = json.Marshal(s.Data)
		if err != nil {
			return domainstatus.StatusInfo[domainstatus.InstanceStatusType]{}, errors.Errorf("marshalling status data: %w", err)
		}
	}

	return domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
		Status:  status,
		Message: s.Message,
		Data:    bytes,
		Since:   s.Since,
	}, nil
}

// decodeInstanceStatus converts a db status info into a core status info.
func decodeInstanceStatus(s domainstatus.StatusInfo[domainstatus.InstanceStatusType]) (status.StatusInfo, error) {
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
