// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
)

type relationUUID struct {
	UUID corerelation.UUID `db:"uuid"`
}

type applicationUUID struct {
	UUID application.ID `db:"application_uuid"`
}

type relationIDAndUUID struct {
	// UUID is the UUID of the relation.
	UUID corerelation.UUID `db:"uuid"`
	// ID is the numeric ID of the relation
	ID int `db:"relation_id"`
}

type relationUUIDAndRole struct {
	// UUID is the unique identifier of the relation.
	UUID string `db:"relation_uuid"`
	// Role is the name of the endpoints role, e.g. provider/requirer/peer.
	Role string `db:"scope"`
}

type relationUnit struct {
	RelationUnitUUID corerelation.UnitUUID `db:"uuid"`
	RelationUUID     corerelation.UUID     `db:"relation_uuid"`
	UnitUUID         unit.UUID             `db:"unit_uuid"`
	InScope          bool                  `db:"in_scope"`
}

type getUnit struct {
	UUID unit.UUID `db:"uuid"`
	Name unit.Name `db:"name"`
}

type getLife struct {
	UUID string     `db:"uuid"`
	Life life.Value `db:"value"`
}

type getUnitApp struct {
	ApplicationUUID application.ID `db:"application_uuid"`
	UnitUUID        unit.UUID      `db:"uuid"`
}

type getScope struct {
	Scope charm.RelationScope `db:"scope"`
}

type getSubordinate struct {
	ApplicationUUID application.ID `db:"application_uuid"`
	Subordinate     bool           `db:"subordinate"`
}

// getPrincipal is used to get the principal application of a unit.
type getPrincipal struct {
	UnitUUID        unit.UUID      `db:"unit_uuid"`
	ApplicationUUID application.ID `db:"application_uuid"`
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
	// EndpointUUID is a unique identifier for the application endpoint
	EndpointUUID corerelation.EndpointUUID `db:"endpoint_uuid"`
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

// String returns a formatted string representation combining
// the ApplicationName and EndpointName of the endpoint.
func (e endpoint) String() string {
	return fmt.Sprintf("%s:%s", e.ApplicationName, e.EndpointName)
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

// setRelationEndpoint represents the mapping to insert a new relation endpoint
// to the table `relation_endpoint`
type setRelationEndpoint struct {
	UUID         corerelation.EndpointUUID `db:"uuid"`
	RelationUUID corerelation.UUID         `db:"relation_uuid"`
	EndpointUUID corerelation.EndpointUUID `db:"endpoint_uuid"`
}

// setRelationStatus represents the structure to insert the status of a relation.
type setRelationStatus struct {
	// RelationUUID is the unique identifier of the relation.
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	// Status indicates the current state of a given relation.
	Status corestatus.Status `db:"status"`
	// UpdatedAt specifies the timestamp of the insertion
	UpdatedAt time.Time `db:"updated_at"`
}
