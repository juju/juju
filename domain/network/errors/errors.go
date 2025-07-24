// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// ContainerAddressesNotSupported is returned when the provider
	// returns false to SupportsContainerAddresses.
	ContainerAddressesNotSupported = errors.ConstError("container addressing not supported")

	// NetNodeNotFound is returned when a network node does not exist.
	NetNodeNotFound = errors.ConstError("network node not found")

	// SpaceAlreadyExists is returned when a space already exists.
	SpaceAlreadyExists = errors.ConstError("space already exists")

	// SpaceNotFound is returned when a space is not found.
	SpaceNotFound = errors.ConstError("space not found")

	// SubnetNotFound is returned when a subnet is not found.
	SubnetNotFound = errors.ConstError("subnet not found")

	// SpaceNameNotValid is returned when a space name is not valid.
	SpaceNameNotValid = errors.ConstError("space name is not valid")

	// AvailabilityZoneNotFound is returned when an availability zone is
	// not found.
	AvailabilityZoneNotFound = errors.ConstError("availability zone not found")

	// SpaceRequirementConflict indicates that negative space constraints for a
	// machine are not satisfiable due to intersection with either its positive
	// space constraints or the app endpoint bindings of units assigned to it.
	SpaceRequirementConflict = errors.ConstError("space requirement conflict")

	// SpaceRequirementsUnsatisfiable indicates that the space requirements for
	// a container or VM cannot be satisfied by the host it was requested to be
	// provisioned on.
	SpaceRequirementsUnsatisfiable = errors.ConstError("space requirements unsatisfiable")
)
