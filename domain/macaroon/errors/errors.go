// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/errors"

const (
	// KeyNotFound describes an error that occurs when a requested root key
	// cannot be found
	KeyNotFound = errors.ConstError("root key not found")

	// KeyAlreadyExists describes an error that occurs when there is a clash
	// when manipulating root keys
	KeyAlreadyExists = errors.ConstError("root key already exists")
)
