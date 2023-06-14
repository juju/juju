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

	// SuspendedReason is an optional message to explain why suspend is true.
	SuspendedReason string

	// Life is the relation life value, eg Alive.
	Life life.Value
}

// RelationStatusChannel is a channel used to notify of changes to
// a relation's life or suspended status.
// This is deprecated; use <-chan []RelationStatusChange instead.
type RelationStatusChannel = <-chan []RelationStatusChange

// RelationStatusWatcher returns a slice of RelationStatusChanges when a
// relation's life or suspended status changes.
type RelationStatusWatcher = Watcher[[]RelationStatusChange]
