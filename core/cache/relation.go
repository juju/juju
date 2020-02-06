// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

// Relation represents a relation in a cached model.
type Relation struct {
	// Resident identifies the relation as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	model   *Model
	details RelationChange
}

func newRelation(model *Model, res *Resident) *Relation {
	return &Relation{
		Resident: res,
		model:    model,
	}
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// Key returns the key of this relation.
func (r *Relation) Key() string {
	return r.details.Key
}

// Endpoints returns the endpoints for this relation.
func (r *Relation) Endpoints() []Endpoint {
	return r.details.Endpoints
}

func (r *Relation) setDetails(details RelationChange) {
	// If this is the first receipt of details, set the removal message.
	if r.removalMessage == nil {
		r.removalMessage = RemoveRelation{
			ModelUUID: details.ModelUUID,
			Key:       details.Key,
		}
	}

	r.setStale(false)
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (r *Relation) copy() Relation {
	cr := *r
	cr.details = cr.details.copy()
	return cr
}
