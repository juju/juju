// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// UnknownKind is raised when the Kind of an ID provided to the annotations
	// state layer is not recognized
	UnknownKind = errors.ConstError("unknown kind")

	// InvalidIDParts is raised when the ID provided to the annotations state
	// layer is invalid.
	InvalidIDParts = errors.ConstError("invalid id parts")

	// NotFound is raised when the annotation is not found.
	NotFound = errors.ConstError("not found")

	// InvalidKey is raised when the key provided to the annotations state
	// layer is invalid.
	InvalidKey = errors.ConstError("invalid key")
)
