// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names/v6"
)

// Unit represents the state of an application unit.
type Unit struct {
}

// Tag returns a name identifying the unit.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (u *Unit) Tag() names.Tag {
	return names.NewUnitTag("no/0")
}
