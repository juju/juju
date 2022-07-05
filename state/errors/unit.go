// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ErrUnitHasSubordinates is a standard error to indicate that a Unit
	// cannot complete an operation to end its life because it still has
	// subordinate applications.
	ErrUnitHasSubordinates = errors.ConstError("unit has subordinates")

	// ErrUnitHasStorageAttachments is a standard error to indicate that
	// a Unit cannot complete an operation to end its life because it still
	// has storage attachments.
	ErrUnitHasStorageAttachments = errors.ConstError("unit has storage attachments")
)
