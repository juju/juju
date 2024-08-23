// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/errors"

var (
	ErrPortRangeConflict = errors.ConstError("port range conflict")
)
