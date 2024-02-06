// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/worker/lifeflag"
)

// ErrRemoved may be returned by some worker started from Manifolds to
// indicate that the unit under management no longer exists.
const ErrRemoved = errors.ConstError("unit removed")

// LifeFilter is used with the lifeflag manifolds -- which do not depend
// on runFlag -- to return appropriate errors for consumption by the
// enclosing dependency.Engine (and/or its IsFatal check).
func LifeFilter(err error) error {
	switch {
	case errors.Is(err, lifeflag.ErrNotFound):
		return ErrRemoved
	case errors.Is(err, lifeflag.ErrValueChanged):
		return dependency.ErrBounce
	}
	return err
}
