// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/relation"
)

type relationUUID struct {
	UUID string `db:"uuid"`
}

type relationIDAndUUID struct {
	// UUID is the UUID of the relation.
	UUID string `db:"uuid"`
	// ID is the numeric ID of the relation
	ID int `db:"relation_id"`
}

// endpoint is used to fetch an endpoint from the database.
type endpoint struct {
	// Endpoint name is the name of the endpoint/relation.
	EndpointName string `db:"endpoint_name"`
	// Role is the name of the endpoints role in the relation.
	Role string `db:"role"`
	// Interface is the name of the interface this endpoint implements.
	Interface string `db:"interface"`
	// Optional marks if this endpoint is required to be in a relation.
	Optional bool `db:"optional"`
	// Capacity defines the maximum number of supported connections to this relation
	// endpoint.
	Capacity int `db:"capacity"`
	// Scope is the name of the endpoints scope.
	Scope string `db:"scope"`
	// ApplicationName is the name of the application this endpoint belongs to.
	ApplicationName string `db:"application_name"`
}

func (e endpoint) toRelationEndpoint() relation.Endpoint {
	return relation.Endpoint{
		ApplicationName: e.ApplicationName,
		Relation: charm.Relation{
			Name:      e.EndpointName,
			Role:      charm.RelationRole(e.Role),
			Interface: e.Interface,
			Optional:  e.Optional,
			Limit:     e.Capacity,
			Scope:     charm.RelationScope(e.Scope),
		},
	}
}
