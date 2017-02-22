// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
)

// Relation represents a juju relation as seen by the firewaller worker.
type Relation struct {
	tag  names.RelationTag
	life params.Life
}

// Tag returns the relation tag.
func (r *Relation) Tag() names.RelationTag {
	return r.tag
}

// Life returns the relation's life cycle value.
func (r *Relation) Life() params.Life {
	return r.life
}
