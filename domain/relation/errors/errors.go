// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AmbiguousRelation is returned when several endpoints match when trying
	// to relate two application
	AmbiguousRelation = errors.ConstError("ambiguous relation")

	// ApplicationIDNotValid describes an error when the application ID is
	// not valid.
	ApplicationIDNotValid = errors.ConstError("application ID not valid")

	// ApplicationNotAlive describes an error that occurs when a relation is
	// added between two application where at least one is not alive
	ApplicationNotAlive = errors.ConstError("application not alive")

	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ApplicationNotFoundForRelation indicates that the application is not part of
	// the relation.
	ApplicationNotFoundForRelation = errors.ConstError("application not found in relation")

	// CompatibleEndpointsNotFound is returned when no matching relation is found when trying
	// to relate two application
	CompatibleEndpointsNotFound = errors.ConstError("no compatible endpoints found between applications")

	// EndpointQuotaLimitExceeded is returned when an operation fails due to
	// exceeding the defined quota limits for an endpoint.
	EndpointQuotaLimitExceeded = errors.ConstError("quota limit exceeded")

	// CannotEnterScopeNotAlive indicates that a relation unit failed to enter
	// its scope due to either the unit or the relation not being Alive.
	CannotEnterScopeNotAlive = errors.ConstError("cannot enter scope, unit or relation not alive")

	// CannotEnterScopeSubordinateNotAlive indicates that a relation unit failed
	// to enter its scope due to a required and pre-existing subordinate unit
	// that is not Alive.
	CannotEnterScopeSubordinateNotAlive = errors.ConstError("cannot enter scope, subordinate unit exists but is not alive")

	// PotentialRelationUnitNotValid describes an error that occurs during
	// EnterScope pre-checks to ensure the created relation unit will be valid.
	//
	// This is not a fatal error. It replaces a boolean return value from a
	// prior implementation.
	PotentialRelationUnitNotValid = errors.ConstError("potential relation unit not valid")

	// RelationEndpointNotFound describes an error that occurs when the specified
	// relation endpoint does not exist.
	RelationEndpointNotFound = errors.ConstError("relation endpoint not found")

	// RelationAlreadyExists indicates an error when attempting to create a relation
	// that already exists between applications.
	RelationAlreadyExists = errors.ConstError("already exists")

	// RelationNotAlive describes an error that occurs when trying to update a
	// relation that is not alive.
	RelationNotAlive = errors.ConstError("relation is not alive")

	// RelationNotFound describes an error that occurs when the specified
	// relation does not exist.
	RelationNotFound = errors.ConstError("relation not found")

	// RelationUUIDNotValid describes an error when the relation UUID is
	// not valid.
	RelationUUIDNotValid = errors.ConstError("relation UUID not valid")

	// RelationKeyNotValid describes an error when the relation key is
	// not valid.
	RelationKeyNotValid = errors.ConstError("relation key not valid")

	// RelationUnitNotFound describes an error that occurs when the specified
	// relation unit does not exist.
	RelationUnitNotFound = errors.ConstError("relation unit not found")

	// UnitUUIDNotValid describes an error when the unit UUID is
	// not valid.
	UnitUUIDNotValid = errors.ConstError("unit UUID not valid")

	// UnitDead describes an error that occurs when trying to update a
	// unit that is dead.
	UnitDead = errors.ConstError("unit is dead")

	// UnitNotFound describes an error when the unit cannot be found.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitPrincipalNotFound describes an error when the principal application
	// of a unit cannot be found.
	UnitPrincipalNotFound = errors.ConstError("unit principal not found")

	// UnitNotInRelation describes an error when the unit specified is not in
	// the relation specified.
	UnitNotInRelation = errors.ConstError("unit not in relation")
)
