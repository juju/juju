// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
)

// This module implements a subset of the interface provided by
// state.Relation, as needed by the uniter API.

// TODO: Only the required calls are added as placeholders,
// the actual implementation will come in a follow-up.

// TODO: Once the relation tags format change from "relation-<id>" to
// "relation-<key>", make the necessary changes here and at
// server-side. This affects the methods Relation() and KeyRelation()
// on uniter.State, as well Id() and String() defined here, and any
// other method taking a relation tag as an argument.

// Relation represents a relation between one or two service
// endpoints.
type Relation struct {
	st   *State
	tag  string
	life params.Life
	// TODO: Add fields.
}

// String returns the relation as a string.
func (r *Relation) String() string {
	_, relId, err := names.ParseTag(r.tag, names.RelationTagKind)
	if err != nil {
		panic(fmt.Sprintf("%q is not a valid relation tag", r.tag))
	}
	return relId
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	// TODO: Convert the relation tag to id and return it.
	panic("not implemented")
}

// Life returns the relation's current life state.
func (r *Relation) Life() params.Life {
	return r.life
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies IsNotFound if the relation has been
// removed.
func (r *Relation) Refresh() error {
	// TODO: Call Uniter.Life(), passing the relation tag as argument.
	// Update r.life accordingly after getting the result.
	panic("not implemented")
}

// Endpoint returns the endpoint of the relation for the service the
// uniter's managed unit belongs to.
func (r *Relation) Endpoint() Endpoint {
	// NOTE: This differs from state.Relation.Endpoint(), because when
	// talking to the API, there's already an authenticated entity - the
	// unit, and we can find out its service name.
	// TODO: Return an Endpoint initialized with the curret auth'ed unit's
	// service name and relevent info.
	panic("not implemented")
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	// TODO: Just create and return a uniter.RelationUnit initialized
	// properly and a nil error.
	panic("not implemented")
}
