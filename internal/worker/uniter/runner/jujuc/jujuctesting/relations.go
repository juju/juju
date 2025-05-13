// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
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
func (r *Relations) SetNewRelation(id int, name string, stub *testhelpers.Stub) *Relation {
	return r.SetNewRelationWithLife(id, name, life.Alive, stub)
}

func (r *Relations) SetNewRelationWithLife(id int, name string, life life.Value, stub *testhelpers.Stub) *Relation {
	if name == "" {
		name = fmt.Sprintf("relation-%d", id)
	}
	rel := &Relation{
		Id:   id,
		Name: name,
		Life: life,
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
func (c *ContextRelations) Relation(id int) (jujuc.ContextRelation, error) {
	c.stub.AddCall("Relation", id)

	r, ok := c.info.Relations[id]
	var err error
	if !ok {
		err = errors.NotFoundf("relation")
	}
	return r, err
}

// RelationIds implements jujuc.ContextRelations.
func (c *ContextRelations) RelationIds() ([]int, error) {
	c.stub.AddCall("RelationIds")

	ids := []int{}
	for id := range c.info.Relations {
		ids = append(ids, id)
	}
	return ids, c.stub.NextErr()
}
