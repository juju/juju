// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	ErrDead = errors.ConstError("not found or dead")

	// IncompatibleBaseError indicates the base selected is not supported by
	// the charm.
	IncompatibleBaseError = errors.ConstError("incompatible base for charm")
)
