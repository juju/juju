// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// This module implements a subset of the interface provided by
// state.Relation, as needed by the uniter API.

// Relation represents a relation between one or two application
// endpoints.
type Relation struct {
	client    *Client
	tag       names.RelationTag
	id        int
	life      life.Value
	suspended bool
	otherApp  string
}

// Tag returns the relation tag.
func (r *Relation) Tag() names.RelationTag {
	return r.tag
}

// String returns the relation as a string.
func (r *Relation) String() string {
	return r.tag.Id()
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different applications.
func (r *Relation) Id() int {
	return r.id
}

// Life returns the relation's current life state.
func (r *Relation) Life() life.Value {
	return r.life
}

// Suspended returns the relation's current suspended status.
func (r *Relation) Suspended() bool {
	return r.suspended
}

// UpdateSuspended updates the in memory value of the
// relation's suspended attribute.
func (r *Relation) UpdateSuspended(suspended bool) {
	r.suspended = suspended
}

// OtherApplication returns the name of the application on the other
// end of the relation (from this unit's perspective).
func (r *Relation) OtherApplication() string {
	return r.otherApp
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// relation has been removed.
func (r *Relation) Refresh(ctx context.Context) error {
	result, err := r.client.relation(ctx, r.tag, r.client.unitTag)
	if err != nil {
		return err
	}
	// NOTE: The status and life cycle information are the only
	// things that can change - id, tag and endpoint
	// information are static.
	r.life = result.Life
	r.suspended = result.Suspended

	return nil
}

// SetStatus updates the status of the relation.
func (r *Relation) SetStatus(ctx context.Context, status relation.Status) error {
	return r.client.setRelationStatus(ctx, r.id, status)
}

func (r *Relation) toCharmRelation(cr params.CharmRelation) charm.Relation {
	return charm.Relation{
		Name:      cr.Name,
		Role:      charm.RelationRole(cr.Role),
		Interface: cr.Interface,
		Optional:  cr.Optional,
		Limit:     cr.Limit,
		Scope:     charm.RelationScope(cr.Scope),
	}
}

// Endpoint returns the endpoint of the relation for the application the
// uniter's managed unit belongs to.
func (r *Relation) Endpoint(ctx context.Context) (*Endpoint, error) {
	// NOTE: This differs from state.Relation.Endpoint(), because when
	// talking to the API, there's already an authenticated entity - the
	// unit, and we can find out its application name.
	result, err := r.client.relation(ctx, r.tag, r.client.unitTag)
	if err != nil {
		return nil, err
	}
	return &Endpoint{r.toCharmRelation(result.Endpoint.Relation)}, nil
}

// Unit returns a RelationUnit for the supplied unitTag.
func (r *Relation) Unit(ctx context.Context, uTag names.UnitTag) (*RelationUnit, error) {
	appName, err := names.UnitApplication(uTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	result, err := r.client.relation(ctx, r.tag, uTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &RelationUnit{
		relation: r,
		unitTag:  uTag,
		appTag:   names.NewApplicationTag(appName),
		endpoint: Endpoint{r.toCharmRelation(result.Endpoint.Relation)},
		client:   r.client,
	}, nil
}
