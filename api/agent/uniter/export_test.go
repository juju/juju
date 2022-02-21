// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
)

var (
	NewSettings = newSettings
)

// CreateUnit creates uniter.Unit for tests.
func CreateUnit(st *State, tag names.UnitTag) *Unit {
	return &Unit{
		st:           st,
		tag:          tag,
		life:         life.Alive,
		resolvedMode: params.ResolvedNone,
	}
}

// CreateRelation creates uniter.Relation for tests.
func CreateRelation(st *State, tag names.RelationTag) *Relation {
	return &Relation{
		st:  st,
		tag: tag,
		id:  666,
	}
}

// CreateRelationUnit creates uniter.RelationUnit for tests.
func CreateRelationUnit(st *State, relationTag names.RelationTag, unitTag names.UnitTag) *RelationUnit {
	return &RelationUnit{
		st:       st,
		unitTag:  unitTag,
		relation: &Relation{tag: relationTag},
	}
}
