// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "github.com/juju/juju/state/multiwatcher"

// RelationUnitChange describes a relation unit change.
type RelationUnitChange struct {
	// Settings is the current settings for the relation unit.
	Settings map[string]interface{}
}

// RelationChange describes changes to a relation.
type RelationChange struct {
	// RelationId is the numeric ID of the relation.
	RelationId int

	// Life is the current lifecycle state of the relation.
	Life multiwatcher.Life

	// ChangedUnits maps unit names to relation unit changes.
	ChangedUnits map[string]RelationUnitChange

	// DepartedUnits contains the names of units that have departed
	// the relation since the last change.
	DepartedUnits []string
}

// ApplicationRelationsChange describes changes to the relations that a service
// is involved in.
type ApplicationRelationsChange struct {
	// ChangedRelations maps relation IDs to relation changes.
	ChangedRelations []RelationChange

	// RemovedRelations contains the IDs of relations removed
	// since the last change.
	RemovedRelations []int
}

// ApplicationRelationsChanges holds a set of ApplicationRelationsChange structures.
type ApplicationRelationsChanges struct {
	Changes []ApplicationRelationsChange
}

// CHECK - PORT params
// ApplicationRelationsChannel is a change channel as described in the CoreWatcher docs.
type ApplicationRelationsChannel <-chan ApplicationRelationsChange

// ApplicationRelationsWatcher is a watcher that reports on changes to relations
// and relation units related to those relations for a specified service.
type ApplicationRelationsWatcher interface {
	CoreWatcher
	Changes() ApplicationRelationsChannel
}
