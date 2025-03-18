// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ApplicationIsDead describes an error that occurs when trying to access
	// an application that is dead.
	ApplicationIsDead = errors.ConstError("application is dead")

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
)
