// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// Invalid indicates that the metadata provided is invalid, meaning that it as several required
	// fields empty.
	Invalid = errors.ConstError("invalid metadata")

	// NotFound is an error constant indicating that the requested metadata could not be found.
	NotFound = errors.ConstError("metadata not found")
)
