// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Relations holds the values for the hook context.
type Relations struct {
	Relations map[int]jujuc.ContextRelation
}

// Reset clears the Relations data.
func (r *Relations) Reset() {
	r.Relations = nil
}

// SetRelation adds the relation to the set of known relations.
func (r *Relations) SetRelation(id int, relCtx jujuc.ContextRelation) {
	if r.Relations == nil {
		r.Relations = make(map[int]jujuc.ContextRelation)
	}
	r.Relations[id] = relCtx
}

// SetNewRelation adds the relation to the set of known relations.
func (r *Relations) SetNewRelation(id int, name string, stub *testing.Stub) *Relation {
	if name == "" {
		name = fmt.Sprintf("relation-%d", id)
	}
	rel := &Relation{
		Id:   id,
		Name: name,
	}
	relCtx := &ContextRelation{info: rel}
	relCtx.stub = stub

	r.SetRelation(id, relCtx)
	return rel
}

// SetRelated adds the provided unit information to the relation.
func (r *Relations) SetRelated(id int, unit string, settings Settings) {
	relation := r.Relations[id].(*ContextRelation).info
	relation.SetRelated(unit, settings)
}

// ContextRelations is a test double for jujuc.ContextRelations.
type ContextRelations struct {
	contextBase
	info *Relations
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
