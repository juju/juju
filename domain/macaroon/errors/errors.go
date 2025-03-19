// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// NotInitialised describes an error that occurs when a requested
	// operation cannot be performed because the macaroon bakery has not been
	// initialised.
	NotInitialised = errors.ConstError("macaroon bakery not initialised")

	// KeyNotFound describes an error that occurs when a requested root key
	// cannot be found
	KeyNotFound = errors.ConstError("root key not found")

	// KeyAlreadyExists describes an error that occurs when there is a clash
	// when manipulating root keys
	KeyAlreadyExists = errors.ConstError("root key already exists")

	// BakeryConfigAlreadyInitialised describes an error that occurs when the
	// bakery config has already been initialised
	BakeryConfigAlreadyInitialised = errors.ConstError("backery config already intialised")
)
