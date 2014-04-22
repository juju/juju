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

// Relation represents a relation between one or two service
// endpoints.
type Relation struct {
	st   *State
	tag  string
	id   int
	life params.Life
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
	return r.id
}

// Life returns the relation's current life state.
func (r *Relation) Life() params.Life {
	return r.life
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// relation has been removed.
func (r *Relation) Refresh() error {
	result, err := r.st.relation(r.tag, r.st.unitTag)
	if err != nil {
		return err
	}
	// NOTE: The life cycle information is the only
	// thing that can change - id, tag and endpoint
	// information are static.
	r.life = result.Life

	return nil
}

// Endpoint returns the endpoint of the relation for the service the
// uniter's managed unit belongs to.
func (r *Relation) Endpoint() (*Endpoint, error) {
	// NOTE: This differs from state.Relation.Endpoint(), because when
	// talking to the API, there's already an authenticated entity - the
	// unit, and we can find out its service name.
	result, err := r.st.relation(r.tag, r.st.unitTag)
	if err != nil {
		return nil, err
	}
	return &Endpoint{result.Endpoint.Relation}, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	if u == nil {
		return nil, fmt.Errorf("unit is nil")
	}
	result, err := r.st.relation(r.tag, u.tag)
	if err != nil {
		return nil, err
	}
	return &RelationUnit{
		relation: r,
		unit:     u,
		endpoint: Endpoint{result.Endpoint.Relation},
		st:       r.st,
	}, nil
}
