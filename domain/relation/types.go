// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	sequence "github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// SequenceNamespace for the sequence table.
const SequenceNamespace = sequence.StaticNamespace("relation")

// GetRelationEndpointUUIDArgs represents the arguments required to retrieve
// the UUID of a relation endpoint.
type GetRelationEndpointUUIDArgs struct {
	// ApplicationID identifies the unique identifier of the application
	// associated with the expected endpoint.
	ApplicationID application.ID

	// RelationUUID represents the unique identifier for the relation associated
	// with the expected endpoint.
	RelationUUID corerelation.UUID
}

// RelationDetails represents the current application's view of a relation.
type RelationDetails struct {
	// Life is the current life value of the relation.
	Life life.Value
	// UUID is the unique identifier of the relation.
	UUID corerelation.UUID
	// ID is the sequential ID of the relation.
	ID int
	// Key is the natural key of the relation.
	Key corerelation.Key
	// Endpoints are the endpoints of the relation.
	Endpoints []Endpoint
}

// RelationDetailsResult represents the current application's view of a
// relation. This struct is used for passing results from state to the service.
type RelationDetailsResult struct {
	// Life is the current life value of the relation.
	Life life.Value
	// UUID is the unique identifier of the relation.
	UUID corerelation.UUID
	// ID is the sequential ID of the relation.
	ID int
	// Endpoints are the endpoints of the relation.
	Endpoints []Endpoint
}

// RelationData holds information about a unit's relation.
type RelationData struct {
	// InScope returns a boolean to indicate whether this unit has successfully
	// joined the relation.
	InScope bool
	// UnitData are the settings for the relation and current unit,
	// set by an individual unit.
	UnitData map[string]interface{} // unit settings
}

// EndpointRelationData holds information about a relation to a given endpoint.
type EndpointRelationData struct {
	// RelationID is the integer internal relation key used by relation hooks
	// to identify a relation.
	RelationID int
	// Endpoint is the name of the endpoint defined in the current application.
	Endpoint string
	// RelatedEndpoint is the name of the endpoint defined in the counterpart application.
	RelatedEndpoint string
	// ApplicationData are the settings for the relation and current application,
	// set by the leader unit.
	ApplicationData map[string]interface{}
	// UnitRelationData are the RelationData for the relation and current unit,
	// set by an individual unit, keyed on the unit name.
	UnitRelationData map[string]RelationData
}

// RelationUnitStatus holds details about scope and suspended status
// for a relation unit.
type RelationUnitStatus struct {
	// Key is the natural key of the relation.
	Key corerelation.Key
	// InScope indicates if the unit has entered the relation scope.
	InScope bool
	// Suspended indicates if the status of this relation is "suspended".
	Suspended bool
}

// RelationUnitStatusResult holds details for a relation unit to return from the
// state layer to the service layer.
type RelationUnitStatusResult struct {
	// Endpoints are the endpoints for this relation the unit is in.
	Endpoints []Endpoint
	// InScope indicates if the unit has entered the relation scope.
	InScope bool
	// Suspended indicates if the status of this relation is "suspended".
	Suspended bool
}

// RelationUnitsWatcher generates signals when units enter or leave
// the scope of a RelationUnit, and changes to the settings of those
// units known to have entered.
type RelationUnitsWatcher interface {
	watcher.Watcher[watcher.RelationUnitsChange]
}

// TODO: uncomment the below types when the methods are implemented,
// copied from state for use.

// RelationScopeChange contains information about units that have
// entered or left a particular scope.
//type RelationScopeChange struct {
//	Entered []string
//	Left    []string
//}

// RelationScopeWatcher observes changes to the set of units
// in a particular relation scope.
type RelationScopeWatcher struct {
	worker.Worker
	//prefix string
	//ignore string
	//out    chan *RelationScopeChange
}

// CandidateEndpointIdentifier is the natural key of a relation endpoint when
// trying to relate two applications. It is used as a parameter for AddRelation,
// as AddRelation will try to infer endpoints from a given ApplicationName if
// there is no EndpointName provided.
// Unlike EndpointIdentifier, this structure cannot be used to uniquely refers
// to an existing endpoint.
type CandidateEndpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string
	// EndpointName is the name of the endpoint. It is optional.
	EndpointName string
}

// String returns the EndpointIdentifier as a concatenated string in the format
// "ApplicationName:EndpointName".
func (e CandidateEndpointIdentifier) String() string {
	if !e.IsFullyQualified() {
		return e.ApplicationName
	}
	return fmt.Sprintf("%s:%s", e.ApplicationName, e.EndpointName)
}

// IsFullyQualified checks if the EndpointIdentifier has a non-empty
// EndpointName, indicating it is fully qualified.
func (e CandidateEndpointIdentifier) IsFullyQualified() bool {
	return len(e.EndpointName) > 0
}

// NewCandidateEndpointIdentifier parses an endpoint string into an EndpointIdentifier
// struct containing application and endpoint names.
// It expects the input format "<application-name>:<endpoint-name>" or
// "<application-name> and returns an error for invalid formats.
func NewCandidateEndpointIdentifier(endpoint string) (CandidateEndpointIdentifier, error) {
	parts := strings.Split(endpoint, ":")
	length := len(parts)
	if length == 0 || length > 2 {
		return CandidateEndpointIdentifier{},
			errors.Errorf("expected endpoint of form <application-name>:<endpoint-name> or <application-name>")
	}
	var endpointName string
	if length > 1 {
		endpointName = parts[1]
	}

	identifier := CandidateEndpointIdentifier{
		ApplicationName: parts[0],
		EndpointName:    endpointName,
	}
	return identifier, nil
}

// CounterpartRole returns the RelationRole that this RelationRole
// can relate to.
// This should remain an internal method because the relation
// model does not guarantee that for every role there will
// necessarily exist a single counterpart role that is sensible
// for basing algorithms upon.
func CounterpartRole(r charm.RelationRole) charm.RelationRole {
	switch r {
	case charm.RoleProvider:
		return charm.RoleRequirer
	case charm.RoleRequirer:
		return charm.RoleProvider
	case charm.RolePeer:
		return charm.RolePeer
	}
	panic(fmt.Errorf("unknown relation role %q", r))
}

// Endpoint represents one endpoint of a relation.
type Endpoint struct {
	ApplicationName string
	charm.Relation
}

// String returns the unique identifier of the relation endpoint.
func (ep Endpoint) String() string {
	return ep.ApplicationName + ":" + ep.Name
}

// CanRelateTo returns whether a relation may be established between e and other.
func (ep Endpoint) CanRelateTo(other Endpoint) bool {
	return ep.ApplicationName != other.ApplicationName &&
		ep.Interface == other.Interface &&
		ep.Role != charm.RolePeer &&
		CounterpartRole(ep.Role) == other.Role
}

// EndpointIdentifier returns the endpoint identifier for this endpoint.
func (ep Endpoint) EndpointIdentifier() corerelation.EndpointIdentifier {
	return corerelation.EndpointIdentifier{
		ApplicationName: ep.ApplicationName,
		EndpointName:    ep.Name,
		Role:            ep.Role,
	}
}

// OtherApplicationForWatcher provides data needed to emit an event from
// the PrincipalLifeSuspendedStatus watcher on other endpoints in a
// relation.
type OtherApplicationForWatcher struct {
	ApplicationID application.ID
	Subordinate   bool
}

// RelationLifeSuspendedData contains the necessary data to notify in
// WatchLifeSuspendedStatus.
type RelationLifeSuspendedData struct {
	EndpointIdentifiers []corerelation.EndpointIdentifier
	Life                life.Value
	Suspended           bool
}

// RelationUnitsChange describes the membership and settings of; or changes to;
// some relation scope.
type RelationUnitsChange struct {

	// Changed holds a set of units that are known to be in scope, and the
	// latest known settings version for each, referenced by unit name.
	Changed map[unit.Name]int64

	// AppChanged holds the latest known settings version for associated
	// applications, referenced by name
	AppChanged map[string]int64

	// Departed holds a set of units that have previously been reported to
	// be in scope, but which no longer are, referenced by unit name.
	Departed []unit.Name
}

// SubordinateCreator creates subordinate units in the database.
type SubordinateCreator interface {
	// CreateSubordinate is the signature of the function used to create units on a
	// subordinate application.
	CreateSubordinate(ctx context.Context, subordinateAppID application.ID, principalUnitName unit.Name) error
}

// GoalStateRelationData contains the necessary data from the relation
// domain to put together a unit's goal state.
type GoalStateRelationData struct {
	EndpointIdentifiers []corerelation.EndpointIdentifier
	Status              corestatus.Status
	Since               *time.Time
}
