// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// RemovalJobTypeNotSupported indicates that
	// a removal job type is not recognised.
	RemovalJobTypeNotSupported = errors.ConstError("removal job type not supported")

	// RemovalJobTypeNotValid indicates that we attempted to process
	// a removal job using logic for an incompatible type.
	RemovalJobTypeNotValid = errors.ConstError("removal job type not valid")

	// RemovalModelRemoved indicates that a model removal job was
	// attempted, but the model was already removed. This should cause the
	// removal worker to remove itself.
	RemovalModelRemoved = errors.ConstError("model removed")

	// EntityStillAlive indicates that an entity for which
	// we are processing a removal job is still alive.
	EntityStillAlive = errors.ConstError("entity still alive")

	// EntityNotDead indicates that an entity for which
	// we are processing a removal job is not dead.
	EntityNotDead = errors.ConstError("entity not dead")

	// RemovalJobIncomplete indicates that the job execution completed without
	// errors, but that it is not complete and expected to be scheduled again
	// later. It is not to be deleted from the removal table.
	RemovalJobIncomplete = errors.ConstError("removal job incomplete")

	// RelationIsCrossModel indicates that a relation cannot be deleted (in this
	// manner) because it is a cross model relation.
	RelationIsCrossModel = errors.ConstError("relation is cross model")

	// UnitsStillInScope indicates that a relation can not be deleted from
	// the database because it has associated relation_unit records.
	UnitsStillInScope = errors.ConstError("units still in relation scope")

	// MachineHasContainers indicates that a machine cannot be deleted because
	// it still hosts containers
	MachineHasContainers = errors.ConstError("machine has containers")

	// MachineHasUnits indicates that a machine cannot be deleted because it
	// still hosts units
	MachineHasUnits = errors.ConstError("machine has units")

	// MachineHasStorge indicates that a machine cannont be deleted because it
	// still has storage.
	MachineHasStorage = errors.ConstError("machine has storage")

	// OfferHasRelations indicates that an offer cannot be deleted because it
	// still has relations
	OfferHasRelations = errors.ConstError("offer has relations")

	// ApplicationHasOfferConnections indicates that an application cannot be
	// deleted because it still has offer connections
	ApplicationHasOfferConnections = errors.ConstError("application has offer connections")

	// ApplicationIsRemoteOfferer indicates that an application cannot be deleted
	// because it is a remote application offerer
	ApplicationIsRemoteOfferer = errors.ConstError("application is remote")

	// ForceRequired indicates that a removal job requires the force flag to
	// be set to true in order to proceed.
	ForceRequired = errors.ConstError("force required for removal job")

	// StorageFulfilmentNotMet indicates that removing a storage entity from
	// the model the fulfilment expectation was not met.
	StorageFulfilmentNotMet = errors.ConstError("storage fulfilment not met")

	// StorageFilesystemNoTombstoned indicates that the filesystem is dead but
	// cannot be removed until it has the tombstone status.
	StorageFilesystemNoTombstone = errors.ConstError("filesystem status is not tombstone")

	// StorageVolumeNoTombstoned indicates that the volume is dead but
	// cannot be removed until it has the tombstone status.
	StorageVolumeNoTombstone = errors.ConstError("volume status is not tombstone")

	// StorageInstanceHasChildren indicates that the storage instance cannot
	// be dead until it has no children.
	StorageInstanceHasChildren = errors.ConstError("storage instance has children")

	// StorageInstanceStillAttached indicates that the storage instance cannot
	// be removed without force as it still has attachments.
	StorageInstanceStillAttached = errors.ConstError("storage instance still attached")
)
