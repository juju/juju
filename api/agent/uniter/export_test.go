// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/life"
)

// CreateUnit creates uniter.Unit for tests.
func CreateUnit(client *Client, tag names.UnitTag) *Unit {
	return &Unit{
		client: client,
		tag:    tag,
		life:   life.Alive,
	}
}

// CreateRelation creates uniter.Relation for tests.
func CreateRelation(client *Client, tag names.RelationTag) *Relation {
	return &Relation{
		client: client,
		tag:    tag,
		id:     666,
	}
}
