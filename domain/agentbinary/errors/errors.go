// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// AlreadyExists defines an error that indicates the agent binary already
	// exists.
	AlreadyExists = errors.ConstError("agent binary already exists")

	// ObjectNotFound defines an error that indicates the binary object
	// associated with the agent binary does not exist.
	ObjectNotFound = errors.ConstError("agent binary object not found")
)
