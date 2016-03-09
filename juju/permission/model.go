// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
)

// ModelAccess defines the permission that a user has on a model.
type ModelAccess string

const (
	// ModelReadAccess allows a user to read a model but not to change it.
	ModelReadAccess ModelAccess = "read"

	// ModelAdminAccess allows a user full control over the model.
	ModelAdminAccess ModelAccess = "admin"
)

// ParseModelAccess parses a string representation of a model access permission
// into its corresponding, effective model access permission type.
func ParseModelAccess(access string) (ModelAccess, error) {
	var fail = ModelAccess("fail")
	switch access {
	case "", "read":
		return ModelReadAccess, nil
	case "write", "admin":
		return ModelAdminAccess, nil
	default:
		return fail, errors.Errorf("invalid model access permission %q", access)
	}
}
