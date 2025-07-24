// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names/v6"
)

// Lifer represents an entity with a life.
type Lifer interface {
	Life() Life
}

// GlobalEntity specifies entity.
type GlobalEntity interface {
	Tag() names.Tag
}
