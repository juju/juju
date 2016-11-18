// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "github.com/juju/juju/state/multiwatcher"

// RemoteRelationUnitChange describes a relation unit change.
type RemoteRelationUnitChange struct {
	// Settings is the current settings for the relation unit.
	Settings map[string]interface{}
}

// RemoteRelationChange describes changes to a relation.
type RemoteRelationChange struct {
	// RelationId is the numeric ID of the relation.
	RelationId int

	// Life is the current lifecycle state of the relation.
	Life multiwatcher.Life

	// ChangedUnits maps unit names to relation unit changes.
	ChangedUnits map[string]RemoteRelationUnitChange

	// DepartedUnits contains the names of units that have departed
	// the relation since the last change.
	DepartedUnits []string
}

// RemoteRelationsChange describes changes to the relations that an application
// is involved in.
type RemoteRelationsChange struct {
	// ChangedRelations maps relation IDs to relation changes.
	ChangedRelations []RemoteRelationChange

	// RemovedRelations contains the IDs of relations removed
	// since the last change.
	RemovedRelations []int
}

// RemoteRelationsChanges holds a set of RemoteRelationsChange structures.
type RemoteRelationsChanges struct {
	Changes []RemoteRelationsChange
}

// CHECK - PORT params
// RemoteRelationsChannel is a change channel as described in the CoreWatcher docs.
type RemoteRelationsChannel <-chan RemoteRelationsChange

// RemoteRelationsWatcher is a watcher that reports on changes to relations
// and relation units related to those relations for a specified application.
type RemoteRelationsWatcher interface {
	CoreWatcher
	Changes() RemoteRelationsChannel
}
