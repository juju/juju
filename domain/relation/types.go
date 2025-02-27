// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/juju/core/application"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
)

// Endpoint represents one endpoint of a relation.
type Endpoint struct {
	ApplicationID application.ID
	charm.Relation
}

// RelationData holds information about a unit's relation.
type RelationData struct {
	// InScope returns a boolean to indicate whether this unit has successfully
	// joined the relation.
	InScope bool `yaml:"in-scope"`
	// UnitData are the settings for the relation and current unit,
	// set by an individual unit.
	UnitData map[string]interface{} `yaml:"data"` // unit settings
}

// EndpointRelationData holds information about a relation to a given endpoint.
type EndpointRelationData struct {
	// RelationID is the integer internal relation key used by relation hooks
	// to identify a relation.
	RelationID int `json:"relation-id"`
	// Endpoint is the name of the endpoint defined in the current application.
	Endpoint string `json:"endpoint"`
	// RelatedEndpoint is the name of the endpoint defined in the counterpart application.
	RelatedEndpoint string `json:"related-endpoint"`
	// ApplicationData are the settings for the relation and current application,
	// set by the leader unit.
	ApplicationData map[string]interface{} `yaml:"application-relation-data"`
	// UnitRelationData are the settings for the relation and current unit,
	// set by an individual unit.
	UnitRelationData map[string]RelationData `json:"unit-relation-data"`
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
