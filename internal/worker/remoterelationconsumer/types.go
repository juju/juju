// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

// RelationUnitChange encapsulates a remote relation event,
// adding the tag of the relation which changed.
type RelationUnitChange struct {
	// ChangedUnits represents the changed units in this relation.
	ChangedUnits []UnitChange

	// DepartedUnits represents the units that have departed in this relation.
	DepartedUnits []int

	// ApplicationSettings represent the updated application-level settings in
	// this relation.
	ApplicationSettings map[string]any
}

// UnitChange represents a change to a single unit in a relation.
type UnitChange struct {
	// UnitId uniquely identifies the remote unit.
	UnitID int

	// Settings is the current settings for the relation unit.
	Settings map[string]any
}
