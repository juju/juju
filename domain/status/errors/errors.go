// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// InvalidStatus describes an error that occurs when the status is not
	// valid.
	InvalidStatus = errors.ConstError("invalid status")

	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ApplicationIsDead describes an error that occurs when trying to access
	// an application that is dead.
	ApplicationIsDead = errors.ConstError("application is dead")

	// RelationNotFound describes an error that occurs when the relation
	// being operated on does not exist.
	RelationNotFound = errors.ConstError("relation not found")

	// RelationStatusTransitionNotValid describes an error that occurs when the
	// current relation status cannot transition to the new relation status.
	RelationStatusTransitionNotValid = errors.ConstError("relation status transition not valid")

	// UnitNotFound describes an error that occurs when the unit being operated
	// on does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitIsDead describes an error that occurs when trying to access
	// an application that is dead.
	UnitIsDead = errors.ConstError("unit is dead")

	// UnitStatusNotFound describes an error that occurs when the unit being
	// operated on does not have a status.
	UnitStatusNotFound = errors.ConstError("unit status not found")

	// UnitNotLeader describes an error that occurs when performing an operation
	// that requires the leader unit with a unit which is not the leader
	UnitNotLeader = errors.ConstError("unit is not the leader")

	// MachineStatusNotFound describes an error that occurs when the machine being
	// operated on does not have a status.
	MachineStatusNotFound = errors.ConstError("machine status not found")

	// FilesystemStatusTransitionNotValid describes an error that occurs when the
	// current filesystem status cannot transition to the new filesystem status.
	FilesystemStatusTransitionNotValid = errors.ConstError("filesystem status transition not valid")

	// VolumeStatusTransitionNotValid describes an error that occurs when the
	// current volume status cannot transition to the new volume status.
	VolumeStatusTransitionNotValid = errors.ConstError("volume status transition not valid")
)
