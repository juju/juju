// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"time"
)

// StatusID represents the status of an entity.
type StatusID interface {
	UnsetStatusType | MachineStatusType | InstanceStatusType
}

// StatusInfo contains the status information for a machine.
type StatusInfo[T StatusID] struct {
	Status  T
	Message string
	Data    []byte
	Since   *time.Time
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
