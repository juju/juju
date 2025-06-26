// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

// ControllerUpgradeBlocker is an error that occurs when a blocker exists
// preventing a controller upgrade from taking place.
type ControllerUpgradeBlocker struct {
	Reason string
}

const (
	// DowngradeNotSupported is an error that occurs when an operation has been
	// asked for that would result in the controller(s) of a cluster having
	// their version downgraded.
	DowngradeNotSupported = errors.ConstError("controller downgrade not supported")

	// MissingControllerBinaries is an error that occurs when an operation
	// cannot be performed because no controller binaries exist for a given
	// version.
	MissingControllerBinaries = errors.ConstError("controller binaries missing")

	// VersionNotSupported is an error that occurs when an operation has been
	// asked for that would result in the controller(s) of a cluster running an
	// unsupported version.
	VersionNotSupported = errors.ConstError("controller version not supported")
)

// Error returns an error message describing why the controller upgrade is
// blocked.
// Implements the [error] interface.
func (e ControllerUpgradeBlocker) Error() string {
	return "controller upgrade blocked: " + e.Reason
}
