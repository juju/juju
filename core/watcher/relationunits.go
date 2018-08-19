// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// UnitSettings specifies the version of some unit's settings in some relation.
type UnitSettings struct {
	Version int64
}

// RelationUnitsChange describes the membership and settings of; or changes to;
// some relation scope.
type RelationUnitsChange struct {

	// Changed holds a set of units that are known to be in scope, and the
	// latest known settings version for each.
	Changed map[string]UnitSettings

	// Departed holds a set of units that have previously been reported to
	// be in scope, but which no longer are.
	Departed []string
}

// RelationUnitsChannel is a change channel as described in the CoreWatcher docs.
//
// It sends a single value representing the current membership of a relation
// scope; and the versions of the settings documents for each; and subsequent
// values representing entry, settings-change, and departure for units in that
// scope.
//
// It feeds the joined-changed-departed logic in worker/uniter, but these events
// do not map 1:1 with hooks.
type RelationUnitsChannel <-chan RelationUnitsChange

// RelationUnitsWatcher conveniently ties a RelationUnitsChannel to the
// worker.Worker that represents its validity.
type RelationUnitsWatcher interface {
	CoreWatcher
	Changes() RelationUnitsChannel
}
