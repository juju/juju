// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
)

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
	// UnitRelationData are the settings for the relation and current unit,
	// set by an individual unit.
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

// Watcher is implemented by all watchers; the actual
// changes channel is returned by a watcher-specific
// Changes method.
type Watcher interface {
	// Kill asks the watcher to stop without waiting for it do so.
	Kill()
	// Wait waits for the watcher to die and returns any
	// error encountered when it was running.
	Wait() error
	// Stop kills the watcher, then waits for it to die.
	Stop() error
	// Err returns any error encountered while the watcher
	// has been running.
	Err() error
}

// RelationUnitsWatcher generates signals when units enter or leave
// the scope of a RelationUnit, and changes to the settings of those
// units known to have entered.
type RelationUnitsWatcher interface {
	Watcher

	Changes() corewatcher.RelationUnitsChannel
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
	Watcher
	//prefix string
	//ignore string
	//out    chan *RelationScopeChange
}

// EndpointIdentifier is the natural key of a relation endpoint.
type EndpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string
	// EndpointName is the name of the endpoint.
	EndpointName string
}

// roleOrder maps RelationRole values to integers to define their order
// of precedence in relation endpoints. This is used to compute the relation's
// natural key.
var roleOrder = map[charm.RelationRole]int{
	charm.RoleRequirer: 0,
	charm.RoleProvider: 1,
	charm.RolePeer:     2,
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

// NaturalKey generates a unique sorted string representation of relation
// endpoints based on their roles and identifiers. It can be used as a natural key
// for relations.
func NaturalKey(endpoints []Endpoint) corerelation.Key {
	eps := slices.SortedFunc(slices.Values(endpoints), func(ep1 Endpoint, ep2 Endpoint) int {
		if ep1.Role != ep2.Role {
			return roleOrder[ep1.Role] - roleOrder[ep2.Role]
		}
		return strings.Compare(ep1.String(), ep2.String())
	})
	endpointNames := make([]string, 0, len(eps))
	for _, ep := range eps {
		endpointNames = append(endpointNames, ep.String())
	}
	return corerelation.Key(strings.Join(endpointNames, " "))
}
