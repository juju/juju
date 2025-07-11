// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// RemovalJobTypeNotSupported indicates that
	// a removal job type is not recognised.
	RemovalJobTypeNotSupported = errors.ConstError("removal job type not supported")

	// RemovalJobTypeNotValid indicates that we attempted to process
	// a removal job using logic for an incompatible type.
	RemovalJobTypeNotValid = errors.ConstError("removal job type not valid")

	// EntityStillAlive indicates that an entity for which
	// we are processing a removal job is still alive.
	EntityStillAlive = errors.ConstError("entity still alive")

	// RemovalJobIncomplete indicates that the job execution completed without
	// errors, but that it is not complete and expected to be scheduled again
	// later. It is not to be deleted from the removal table.
	RemovalJobIncomplete = errors.ConstError("removal job incomplete")

	// UnitsStillInScope indicates that a relation can not be deleted from
	// the database because it has associated relation_unit records.
	UnitsStillInScope = errors.ConstError("units still in relation scope")

	// MachineHasContainers indicates that a machine cannot be deleted because
	// it still hosts containers
	MachineHasContainers = errors.ConstError("machine has containers")

	// MachineHasUnits indicates that a machine cannot be deleted because it
	// still hosts units
	MachineHasUnits = errors.ConstError("machine has units")
)
