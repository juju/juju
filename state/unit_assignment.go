// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// UnitAssignment represents a staged unit assignment.
type UnitAssignment struct {
	// Unit is the ID of the unit to be assigned.
	Unit string

	// Scope is the placement scope to apply to the unit.
	Scope string

	// Directive is the placement directive to apply to the unit.
	Directive string
}
