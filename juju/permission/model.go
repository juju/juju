// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
)

// ModelAccess defines the permission that a user has on a model.
type ModelAccess int

const (
	_ = iota

	// ModelReadAccess allows a user to read a model but not to change it.
	ModelReadAccess ModelAccess = iota

	// ModelWriteAccess allows a user write access to the model.
	ModelWriteAccess ModelAccess = iota
)

// ParseModelAccess parses a user-facing string representation of a model
// access permission into a logical representation.
func ParseModelAccess(access string) (ModelAccess, error) {
	var fail = ModelAccess(0)
	switch access {
	case "read":
		return ModelReadAccess, nil
	case "write":
		return ModelWriteAccess, nil
	default:
		return fail, errors.Errorf("invalid model access permission %q", access)
	}
}
