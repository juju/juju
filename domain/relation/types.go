// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corewatcher "github.com/juju/juju/core/watcher"
	internalrelation "github.com/juju/juju/internal/relation"
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
	Life     life.Value
	UUID     corerelation.UUID
	ID       int
	Key      string
	Endpoint []internalrelation.Endpoint
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
	Key       internalrelation.Key
	InScope   bool
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
