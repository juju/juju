// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/network"
)

const ManualInstancePrefix = "manual:"

// ExportMachine represents a machine that is being exported to another
// controller.
type ExportMachine struct {
	Name         machine.Name
	UUID         machine.UUID
	Nonce        string
	PasswordHash string
	Placement    string
	Base         string
	InstanceID   string
}

// CreateMachineArgs contains arguments for creating a machine.
type CreateMachineArgs struct {
	MachineUUID machine.UUID
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform
	Nonce       *string
}

// AddMachineArgs contains arguments for adding a machine.
type AddMachineArgs struct {
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform
	Nonce       *string

	// HardwareCharacteristics contains the hardware characteristics for a manually provisioned machine.
	HardwareCharacteristics instance.HardwareCharacteristics
}

type PlaceMachineArgs struct {
	Constraints constraints.Constraints
	Directive   deployment.Placement
	Platform    deployment.Platform

	// MachineUUID represents the uuid to use for any new machine that is
	// created as part of this placement.
	MachineUUID machine.UUID

	// NetNodeUUID represents either the netnode uuid of an existing machine in
	// the model or a new netnode uuid to assign to a new machine being created
	// in the model.
	// NetNodeUUID represents the uuid of the new machines net node that is
	// created as part of this placement.
	NetNodeUUID network.NetNodeUUID

	Nonce *string

	// HardwareCharacteristics contains the hardware characteristics for a manually provisioned machine.
	HardwareCharacteristics instance.HardwareCharacteristics
}

// PollingInfo contains information about a machine that is being polled.
// It is used to get an eventual instanceID  from the provider and existing
// device count detected by the machiner.
type PollingInfo struct {
	MachineUUID         machine.UUID
	MachineName         machine.Name
	InstanceID          instance.Id
	ExistingDeviceCount int
}

// PollingInfos is a slice of PollingInfo.
type PollingInfos []PollingInfo
