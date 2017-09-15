// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/core/life"
)

// RelationStatusChange describes changes to some relation.
type RelationStatusChange struct {
	// Key is the relation key of the changed relation.
	Key string

	// Suspended is the suspended status of the relation.
	Suspended bool

	// Life is the relation life value, eg Alive.
	Life life.Value
}

// RelationStatusChannel is a channel used to notify of changes to
// a relation's life or suspended status.
type RelationStatusChannel <-chan []RelationStatusChange

// RelationStatusWatcher conveniently ties a RelationStatusChannel to the
// worker.Worker that represents its validity.
type RelationStatusWatcher interface {
	CoreWatcher
	Changes() RelationStatusChannel
}
