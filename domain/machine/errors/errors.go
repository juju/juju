// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// MachineIsDead describes an error that occurs when the machine being
	// operated on is considered dead.
	MachineIsDead = errors.ConstError("machine is dead")

	// MachineNotAlive describes an error that occurs when the machine being
	// operated on is considered not alive.
	MachineNotAlive = errors.ConstError("machine is not alive")

	// MachineNotFound describes an error that occurs when the machine being
	// operated on does not exist.
	MachineNotFound = errors.ConstError("machine not found")

	// AvailabilityZoneNotFound describes an error that occurs when the required
	// availability zone does not exist.
	AvailabilityZoneNotFound = errors.ConstError("availability zone not found")

	// NotProvisioned describes an error that occurs when the machine being
	// operated on is not provisioned yet.
	NotProvisioned = errors.ConstError("machine not provisioned")

	// InvalidContainerType describes an error that can occur when a container
	// type has been used that isn't understood by the Juju controller.
	// Container types can currently be found in
	// [github.com/juju/juju/core/instance.ContainerType]
	InvalidContainerType = errors.ConstError("invalid container type")

	// GrandParentNotSupported describes an error that occurs when the operation
	// found a grandparent machine, as it is not currently supported.
	GrandParentNotSupported = errors.ConstError("grandparent machine are not supported currently")

	// MachineAlreadyExists describes an error that occurs when creating a
	// machine if a machine with the same name already exists.
	MachineAlreadyExists = errors.ConstError("machine already exists")

	// MachineHasNoParent describes an error that occurs when a machine has no
	// parent.
	MachineHasNoParent = errors.ConstError("machine has no parent")

	// MachineCloudInstanceAlreadyExists describes an error that occurs
	// when adding cloud instance on a machine that already exists.
	MachineCloudInstanceAlreadyExists = errors.ConstError("machine cloud instance already exists")
)
