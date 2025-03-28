// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
)

type relationUUID struct {
	UUID string `db:"uuid"`
}

type relationIDAndUUID struct {
	// UUID is the UUID of the relation.
	UUID corerelation.UUID `db:"uuid"`
	// ID is the numeric ID of the relation
	ID int `db:"relation_id"`
}

// relationStatus represents the status of a relation
// from v_relation_status
type relationStatus struct {
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	Status       string            `db:"status"`
	Reason       string            `db:"suspended_reason"`
	Since        time.Time         `db:"updated_at"`
}

type relationUUIDAndRole struct {
	// UUID is the unique identifier of the relation.
	UUID string `db:"relation_uuid"`
	// Role is the name of the endpoints role, e.g. provider/requirer/peer.
	Role string `db:"scope"`
}

// relationForDetails represents the structure for retrieving
// relation details from the database.
type relationForDetails struct {
	// UUID uniquely identifies the relation.
	UUID corerelation.UUID `db:"uuid"`
	// ID is the numerical identifier of the relation.
	ID int `db:"relation_id"`
	// Life indicates the state of life for the relation.
	Life life.Value `db:"value"`
}

// endpointIdentifier is an identifier for a relation endpoint.
type endpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string `db:"application_name"`
	// EndpointName is the name of the endpoint.
	EndpointName string `db:"endpoint_name"`
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

// toRelationEndpoint converts an endpoint read out of the database to a
// relation.Endpoint.
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
