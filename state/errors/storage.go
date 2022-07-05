// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// StorageAttachedError reports whether or not the given error was caused
	// by an operation on storage that should not be, but is, attached.
	StorageAttachedError = errors.ConstError("storage is attached")
)
