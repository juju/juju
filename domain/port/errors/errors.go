// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// UnitNotFound describes an error that occurs when the unit
	// being operated on does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// PortRangeConflict describes an error that occurs when a user tries to open
	// or close a port range overlaps with another.
	PortRangeConflict = errors.ConstError("port range conflict")

	// InvalidEndpoint describes an error that occurs when a user trying to open
	// or close a port range with an endpoint which does not exist on the unit.
	InvalidEndpoint = errors.ConstError("invalid endpoint(s)")
)
