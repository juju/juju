// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names/v6"
)

// Entity represents any entity that can be returned
// by State.FindEntity. All entities have a tag.
type Entity interface {
	Tag() names.Tag
}

// Lifer represents an entity with a life.
type Lifer interface {
	Life() Life
}

// GlobalEntity specifies entity.
type GlobalEntity interface {
	Tag() names.Tag
}
