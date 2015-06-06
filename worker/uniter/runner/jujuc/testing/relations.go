// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Relations holds the values for the hook context.
type Relations struct {
	Relations map[int]jujuc.ContextRelation
}

// Set adds the relation to the set of known relations.
func (r *Relations) Set(id int, relCtx jujuc.ContextRelation) {
	if r.Relations == nil {
		r.Relations = make(map[int]jujuc.ContextRelation)
	}
	r.Relations[id] = relCtx
}

// ContextRelations is a test double for jujuc.ContextRelations.
type ContextRelations struct {
	contextBase
	info *Relations
}

func (c *ContextRelations) setRelation(id int, name string) *Relation {
	if name == "" {
		name = fmt.Sprintf("relation-%d", id)
	}
	rel := &Relation{
		Id:   id,
		Name: name,
	}
	relCtx := &ContextRelation{info: rel}
	relCtx.stub = c.stub

	c.info.Set(id, relCtx)
	return rel
}

// Relation implements jujuc.ContextRelations.
func (c *ContextRelations) Relation(id int) (jujuc.ContextRelation, bool) {
	c.stub.AddCall("Relation", id)
	c.stub.NextErr()

	r, found := c.info.Relations[id]
	return r, found
}

// RelationIds implements jujuc.ContextRelations.
func (c *ContextRelations) RelationIds() []int {
	c.stub.AddCall("RelationIds")
	c.stub.NextErr()

	ids := []int{}
	for id := range c.info.Relations {
		ids = append(ids, id)
	}
	return ids
}
