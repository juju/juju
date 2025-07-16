// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// assignUnitDoc is a document that temporarily stores unit assignment
// information created during service creation until the unitassigner worker can
// come along and use it.
type assignUnitDoc struct {
	// DocId is the unique id of the document, which is also the unit id of the
	// unit to be assigned.
	DocId string `bson:"_id"`

	// Scope is the placement scope to apply to the unit.
	Scope string `bson:"scope"`

	// Directive is the placement directive to apply to the unit.
	Directive string `bson:"directive"`
}

// UnitAssignment represents a staged unit assignment.
type UnitAssignment struct {
	// Unit is the ID of the unit to be assigned.
	Unit string

	// Scope is the placement scope to apply to the unit.
	Scope string

	// Directive is the placement directive to apply to the unit.
	Directive string
}
