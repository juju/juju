// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"time"

	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
)

// StatusID represents the status of an entity.
type StatusID interface {
	K8sPodStatusType | RelationStatusType |
		UnitAgentStatusType | WorkloadStatusType |
		StorageFilesystemStatusType | StorageVolumeStatusType |
		UnsetStatusType | MachineStatusType | InstanceStatusType
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

// UnitStatusInfo holds details about the status of a unit agent. This
// indicates if the unit agent is present and currently active in the model.
type UnitStatusInfo[T UnitStatusID] struct {
	StatusInfo[T]
	// Present is true if the unit agent logged into the API server.
	Present bool
}

// K8sPodStatusType represents the status of a cloud container
// as recorded in the k8s_pod_status_value lookup table.
type K8sPodStatusType int

const (
	K8sPodStatusUnset K8sPodStatusType = iota
	K8sPodStatusWaiting
	K8sPodStatusBlocked
	K8sPodStatusRunning
)

// EncodeK8sPodStatus encodes a K8sPodStatusType into it's integer
// id, as recorded in the k8s_pod_status_value lookup table.
func EncodeK8sPodStatus(s K8sPodStatusType) (int, error) {
	switch s {
	case K8sPodStatusUnset:
		return 0, nil
	case K8sPodStatusWaiting:
		return 1, nil
	case K8sPodStatusBlocked:
		return 2, nil
	case K8sPodStatusRunning:
		return 3, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeK8sPodStatus decodes a K8sPodStatusType from it's integer
// id, as recorded in the k8s_pod_status_value lookup table.
func DecodeK8sPodStatus(s int) (K8sPodStatusType, error) {
	switch s {
	case 0:
		return K8sPodStatusUnset, nil
	case 1:
		return K8sPodStatusWaiting, nil
	case 2:
		return K8sPodStatusBlocked, nil
	case 3:
		return K8sPodStatusRunning, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// ModelStatusContext describes the context used to determine the status of a model.
type ModelStatusContext struct {
	// IsDestroying indicates if the model is in the process of being destroyed.
	IsDestroying bool
	// IsMigrating indicates if the model is in the process of being migrated.
	IsMigrating bool
	// HasInvalidCloudCredential indicates if the model's cloud credential is invalid.
	HasInvalidCloudCredential bool
	// InvalidCloudCredentialReason explains why the model's cloud credential is invalid.
	InvalidCloudCredentialReason string
}

// ModelStatusInfo represents the basic information about a model for the
// purpose of reporting its status.
type ModelStatusInfo struct {
	// Type is the type of the model in question.
	Type coremodel.ModelType
}

type RelationStatusInfo struct {
	RelationUUID corerelation.UUID
	RelationID   int
	StatusInfo   StatusInfo[RelationStatusType]
}

// RelationStatusType represents the status of a relation as recorded in the
// relation_status_value lookup table.
type RelationStatusType int

const (
	RelationStatusTypeJoining RelationStatusType = iota
	RelationStatusTypeJoined
	RelationStatusTypeBroken
	RelationStatusTypeSuspending
	RelationStatusTypeSuspended
	RelationStatusTypeError
)

// EncodeRelationStatus encodes a RelationStatusType from into it's integer id, as
// recorded in the relation_status_value lookup table.
func EncodeRelationStatus(s RelationStatusType) (int, error) {
	switch s {
	case RelationStatusTypeJoining:
		return 0, nil
	case RelationStatusTypeJoined:
		return 1, nil
	case RelationStatusTypeBroken:
		return 2, nil
	case RelationStatusTypeSuspending:
		return 3, nil
	case RelationStatusTypeSuspended:
		return 4, nil
	case RelationStatusTypeError:
		return 5, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeRelationStatus decodes a RelationStatusType from it's integer id, as
// recorded in the relation_status_value lookup table.
func DecodeRelationStatus(s int) (RelationStatusType, error) {
	switch s {
	case 0:
		return RelationStatusTypeJoining, nil
	case 1:
		return RelationStatusTypeJoined, nil
	case 2:
		return RelationStatusTypeBroken, nil
	case 3:
		return RelationStatusTypeSuspending, nil
	case 4:
		return RelationStatusTypeSuspended, nil
	case 5:
		return RelationStatusTypeError, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// RelationStatusTransitionValid returns the error
// [statuserror.RelationStatusTransitionNotValid] if the transition from the
// current relation status to the new relation status is not valid.
func RelationStatusTransitionValid(current, new StatusInfo[RelationStatusType]) error {
	if current.Status != new.Status {
		validTransition := true
		switch new.Status {
		case RelationStatusTypeBroken:
		case RelationStatusTypeSuspending:
			validTransition = current.Status != RelationStatusTypeBroken && current.Status != RelationStatusTypeSuspended
		case RelationStatusTypeJoining:
			validTransition = current.Status != RelationStatusTypeBroken && current.Status != RelationStatusTypeJoined
		case RelationStatusTypeJoined, RelationStatusTypeSuspended:
			validTransition = current.Status != RelationStatusTypeBroken
		case RelationStatusTypeError:
			if new.Message == "" {
				return errors.Errorf("cannot set status %q without message", new.Status)
			}
		default:
			return errors.Errorf("cannot set invalid status %q", new.Status)
		}
		if !validTransition {
			return errors.Errorf(
				"cannot set status %q when relation has status %q: %w",
				new.Status, current.Status, statuserrors.RelationStatusTransitionNotValid,
			)
		}
	}
	return nil
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

// EncodeAgentStatus encodes a UnitAgentStatusType into it's integer id, as
// recorded in the unit_agent_status_value lookup table.
func EncodeAgentStatus(s UnitAgentStatusType) (int, error) {
	switch s {
	case UnitAgentStatusAllocating:
		return 0, nil
	case UnitAgentStatusExecuting:
		return 1, nil
	case UnitAgentStatusIdle:
		return 2, nil
	case UnitAgentStatusError:
		return 3, nil
	case UnitAgentStatusFailed:
		return 4, nil
	case UnitAgentStatusLost:
		return 5, nil
	case UnitAgentStatusRebooting:
		return 6, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeAgentStatus decodes a UnitAgentStatusType from it's integer id, as
// recorded in the unit_agent_status_value lookup table.
func DecodeAgentStatus(s int) (UnitAgentStatusType, error) {
	switch s {
	case 0:
		return UnitAgentStatusAllocating, nil
	case 1:
		return UnitAgentStatusExecuting, nil
	case 2:
		return UnitAgentStatusIdle, nil
	case 3:
		return UnitAgentStatusError, nil
	case 4:
		return UnitAgentStatusFailed, nil
	case 5:
		return UnitAgentStatusLost, nil
	case 6:
		return UnitAgentStatusRebooting, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

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

// EncodeWorkloadStatus encodes a WorkloadStatusType into it's integer id, as
// recorded in the workload_status_value lookup table.
func EncodeWorkloadStatus(s WorkloadStatusType) (int, error) {
	switch s {
	case WorkloadStatusUnset:
		return 0, nil
	case WorkloadStatusUnknown:
		return 1, nil
	case WorkloadStatusMaintenance:
		return 2, nil
	case WorkloadStatusWaiting:
		return 3, nil
	case WorkloadStatusBlocked:
		return 4, nil
	case WorkloadStatusActive:
		return 5, nil
	case WorkloadStatusTerminated:
		return 6, nil
	case WorkloadStatusError:
		return 7, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// DecodeWorkloadStatus decodes a WorkloadStatusType from it's integer id, as
// recorded in the workload_status_value lookup table.
func DecodeWorkloadStatus(s int) (WorkloadStatusType, error) {
	switch s {
	case 0:
		return WorkloadStatusUnset, nil
	case 1:
		return WorkloadStatusUnknown, nil
	case 2:
		return WorkloadStatusMaintenance, nil
	case 3:
		return WorkloadStatusWaiting, nil
	case 4:
		return WorkloadStatusBlocked, nil
	case 5:
		return WorkloadStatusActive, nil
	case 6:
		return WorkloadStatusTerminated, nil
	case 7:
		return WorkloadStatusError, nil
	default:
		return -1, errors.Errorf("unknown status %d", s)
	}
}

// UnsetStatusType represents the status of an entity that has not been set.
type UnsetStatusType int

const (
	UnsetStatus UnsetStatusType = iota
)

// MachineStatusType represents the status of a machine
// as recorded in the machine_status_value lookup table.
type MachineStatusType int

const (
	MachineStatusStarted MachineStatusType = iota
	MachineStatusStopped
	MachineStatusError
	MachineStatusPending
	MachineStatusDown
	MachineStatusUnknown
)

// DecodeMachineStatus decodes a string representation of a machine status
// into a MachineStatusType. It returns an error if the string
// does not match any known status.
func DecodeMachineStatus(s string) (MachineStatusType, error) {
	var result MachineStatusType
	switch s {
	case "error":
		result = MachineStatusError
	case "started":
		result = MachineStatusStarted
	case "pending":
		result = MachineStatusPending
	case "stopped":
		result = MachineStatusStopped
	case "down":
		result = MachineStatusDown
	case "":
		result = MachineStatusUnknown
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// EncodeMachineStatus encodes a MachineStatusType into its
// corresponding integer representation. It returns an error if the status
// is unknown.
func EncodeMachineStatus(s MachineStatusType) (int, error) {
	var result int
	switch s {
	case MachineStatusError:
		result = 0
	case MachineStatusStarted:
		result = 1
	case MachineStatusPending:
		result = 2
	case MachineStatusStopped:
		result = 3
	case MachineStatusDown:
		result = 4
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// InstanceStatusType represents the status of an instance
// as recorded in the instance_status_value lookup table.
type InstanceStatusType int

const (
	InstanceStatusUnset InstanceStatusType = iota
	InstanceStatusPending
	InstanceStatusAllocating
	InstanceStatusRunning
	InstanceStatusProvisioningError
)

// EncodeCloudInstanceStatus encodes a InstanceStatusType into
// its corresponding integer representation. It returns an error if the status
// is unknown.
func EncodeCloudInstanceStatus(s InstanceStatusType) (int, error) {
	var result int
	switch s {
	case InstanceStatusUnset:
		result = 0
	case InstanceStatusPending:
		result = 1
	case InstanceStatusAllocating:
		result = 2
	case InstanceStatusRunning:
		result = 3
	case InstanceStatusProvisioningError:
		result = 4
	default:
		return -1, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// DecodeCloudInstanceStatus decodes a string representation of a cloud instance
// status into a InstanceStatusType. It returns an error if the
// string does not match any known status.
func DecodeCloudInstanceStatus(s string) (InstanceStatusType, error) {
	var result InstanceStatusType
	switch s {
	case "unknown", "":
		result = InstanceStatusUnset
	case "pending":
		result = InstanceStatusPending
	case "allocating":
		result = InstanceStatusAllocating
	case "running":
		result = InstanceStatusRunning
	case "provisioning error":
		result = InstanceStatusProvisioningError
	default:
		return 0, errors.Errorf("unknown status %q", s)
	}
	return result, nil
}

// UnitWorkloadAgentStatus holds details about the workload and agent status of a unit.
type UnitWorkloadAgentStatus struct {
	WorkloadStatus StatusInfo[WorkloadStatusType]
	AgentStatus    StatusInfo[UnitAgentStatusType]
	// Present is true if the unit agent logged into the API server.
	Present bool
}

// FullUnitStatus holds details about the workload, agent and container status of a unit.
type FullUnitStatus struct {
	WorkloadStatus StatusInfo[WorkloadStatusType]
	AgentStatus    StatusInfo[UnitAgentStatusType]
	K8sPodStatus   StatusInfo[K8sPodStatusType]
	// Present is true if the unit agent logged into the API server.
	Present bool
}

// UnitWorkloadStatuses represents the workload statuses of a collection of units.
// The statuses are indexed by unit name.
type UnitWorkloadStatuses map[unit.Name]UnitStatusInfo[WorkloadStatusType]

// UnitAgentStatuses represents the agent statuses of a collection of units.
// The statuses are indexed by unit name.
type UnitAgentStatuses map[unit.Name]StatusInfo[UnitAgentStatusType]

// UnitK8sPodStatuses represents the cloud container statuses of a collection
// of units. The statuses are indexed by unit name.
type UnitK8sPodStatuses map[unit.Name]StatusInfo[K8sPodStatusType]

// UnitWorkloadAgentStatuses represents the workload and agent statuses of a
// collection of units.
type UnitWorkloadAgentStatuses map[unit.Name]UnitWorkloadAgentStatus

// FullUnitStatuses represents the workload, agent and container statuses of a
// collection of units.
type FullUnitStatuses map[unit.Name]FullUnitStatus
