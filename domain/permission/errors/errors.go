// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/errors"

const (
	// AlreadyExists describes an error that occurs when the user being
	// created already exists.
	AlreadyExists = errors.ConstError("permission already exists")
)
