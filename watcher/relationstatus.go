// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
)

// RelationStatusChange describes changes to some relation.
type RelationStatusChange struct {
	// Key is the relation key of the changed relation.
	Key string

	// Status is the status of the relation, eg Active.
	Status relation.Status

	// Life is the relation life value, eg Alive.
	Life life.Value
}

// RelationStatusChannel is a channel used to notify of changes to a relation's status.
type RelationStatusChannel <-chan []RelationStatusChange

// RelationStatusWatcher conveniently ties a RelationStatusChannel to the
// worker.Worker that represents its validity.
type RelationStatusWatcher interface {
	CoreWatcher
	Changes() RelationStatusChannel
}
