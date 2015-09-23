// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// assignUnitDoc is a document that temporarily stores unit assignment
// information created during srevice creation until the unitassigner worker can
// come along and use it.
type assignUnitDoc struct {
	// DocId is the unique id of the document.
	DocId string `bson:"_id"`

	// EnvUUID is the environment identifier.
	EnvUUID string `bson:"env-uuid"`

	// Unit is the if of the unit to assign to a machine.
	Unit string `bson:"unit"`

	// Scope is the placement scope to apply to the unit.
	Scope string `bson:"scope`

	// Directive is the placement directive to apply to the unit.
	Directive string `bson:"scope`
}

// UnitAssignmentResult is the result of running a staged unit assignment.
type UnitAssignmentResult struct {
	Unit  string
	Error error
}
