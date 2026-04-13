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
	// Hostname is the hostname of the machine.
	Hostname string

	// MachineUUID represents the uuid to use for the new machine being created.
	MachineUUID machine.UUID

	// Constraints are the constraints for the machine being created.
	Constraints constraints.Constraints

	// Directive is the placement directive for the machine being created. This
	// indicates where the machine should be placed in the model.
	Directive deployment.Placement

	// Platform is the deployment platform for the machine being created.
	Platform deployment.Platform

	// NetNodeUUID represents the uuid of the new machines net node that is
	// created.
	NetNodeUUID network.NetNodeUUID

	// Nonce is an optional nonce to associate with the machine being created.
	Nonce *string
}

// AddMachineArgs contains arguments for adding a machine.
type AddMachineArgs struct {
	// Constraints are the constraints for the machine being placed.
	Constraints constraints.Constraints

	// Directive is the placement directive for the machine being placed. This
	// indicates where the machine should be placed in the model.
	Directive deployment.Placement

	// Platform is the deployment platform for the machine being added.
	Platform deployment.Platform

	// Nonce is an optional nonce to associate with the machine being added.
	Nonce *string

	// InstanceID is an optional instance ID when creating the machine. This
	// primarily used for manual machines and it's the provider instance ID for
	// the machine being placed.
	InstanceID *instance.Id

	// HardwareCharacteristics contains the hardware characteristics for a
	// manually provisioned machine.
	HardwareCharacteristics instance.HardwareCharacteristics
}

// PlaceMachineArgs contains arguments for placing a machine.

type PlaceMachineArgs struct {
	// Constraints are the constraints for the machine being placed.
	Constraints constraints.Constraints

	// Directive is the placement directive for the machine being placed. This
	// indicates where the machine should be placed in the model.
	Directive deployment.Placement

	// Platform is the deployment platform for the machine being placed.
	Platform deployment.Platform

	// MachineUUID represents the uuid to use for any new machine that is
	// created as part of this placement.
	MachineUUID machine.UUID

	// NetNodeUUID represents either the netnode uuid of an existing machine in
	// the model or a new netnode uuid to assign to a new machine being created
	// in the model.
	// NetNodeUUID represents the uuid of the new machines net node that is
	// created as part of this placement.
	NetNodeUUID network.NetNodeUUID

	// Nonce is an optional nonce to associate with the machine being placed.
	Nonce *string

	// InstanceID is an optional instance ID when creating the machine. This
	// primarily used for manual machines and it's the provider instance ID for
	// the machine being placed.
	InstanceID *instance.Id

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
