// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// UnitNotFound is the error returned when a unit is not found.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitAgentStatusNotFound is the error returned when a unit's agent status
	// is not found.
	UnitAgentStatusNotFound = errors.ConstError("unit agent status not found")

	// UnitNotResolved is the error returned when a unit is expected to have a resolved
	// marker but does not.
	UnitNotResolved = errors.ConstError("unit not resolved")

	// UnitNotInErrorState is the error returned when a unit that is expected to
	// be in error state is not in error state.
	UnitNotInErrorState = errors.ConstError("unit is not in error state")
)
