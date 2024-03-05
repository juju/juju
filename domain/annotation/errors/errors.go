// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// UnknownKind is raised when the Kind of an ID provided to the annotations state
	// layer is not recognized
	UnknownKind = errors.ConstError("unknown kind")
)
