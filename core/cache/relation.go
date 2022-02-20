// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import "fmt"

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

// Report returns information that is used in the dependency engine report.
func (r *Relation) Report() map[string]interface{} {
	details := r.details

	endpoints := make([]string, len(details.Endpoints))
	for k, ep := range details.Endpoints {
		endpoints[k] = fmt.Sprintf("%s:%s", ep.Application, ep.Name)
	}

	return map[string]interface{}{
		"key":       details.Key,
		"endpoints": endpoints,
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
	r.setRemovalMessage(RemoveRelation{
		ModelUUID: details.ModelUUID,
		Key:       details.Key,
	})
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (r *Relation) copy() Relation {
	cr := *r
	cr.details = cr.details.copy()
	return cr
}
