// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port

import "github.com/juju/juju/internal/errors"

var (
	ErrPortRangeConflict = errors.ConstError("port range conflict")
)
