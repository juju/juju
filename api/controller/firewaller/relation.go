// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/names/v4"
)

// Relation represents a juju relation as seen by the firewaller worker.
type Relation struct {
	tag  names.RelationTag
	life life.Value
}

// Tag returns the relation tag.
func (r *Relation) Tag() names.RelationTag {
	return r.tag
}

// Life returns the relation's life cycle value.
func (r *Relation) Life() life.Value {
	return r.life
}
