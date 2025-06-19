// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// NotFound describes an error that occurs when a controller cannot be
	// found.
	NotFound = errors.ConstError("controller not found")

	// ControllerAddressNotValid describes an error that occurs when a
	// controller address is not valid.
	ControllerAddressNotValid = errors.ConstError("controller address not valid")

	// EmptyControllerIDs describes an error that occurs when no controller IDs
	// are found.
	EmptyControllerIDs = errors.ConstError("no controller IDs found")

	// EmptyAPIAddresses describes an error that occurs when no API addresses
	// are found.
	EmptyAPIAddresses = errors.ConstError("no API addresses found")

	// InvalidPassword describes an error that occurs when the password is not
	// valid.
	InvalidPassword = errors.ConstError("invalid password")
)
