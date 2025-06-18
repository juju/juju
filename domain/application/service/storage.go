// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// StorageState describes retrieval and persistence methods for
// storage related interactions.
type StorageState interface {
	// GetStorageUUIDByID returns the UUID for the specified storage, returning an error
	// satisfying [github.com/juju/juju/domain/storage/errors.StorageNotFound] if the storage doesn't exist.
	GetStorageUUIDByID(ctx context.Context, storageID corestorage.ID) (corestorage.UUID, error)

	// AttachStorage attaches the specified storage to the specified unit.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.StorageAlreadyAttached]: when the attachment already exists.
	// - [github.com/juju/juju/domain/application/errors.FilesystemAlreadyAttached]: when the filesystem is already attached.
	// - [github.com/juju/juju/domain/application/errors.VolumeAlreadyAttached]: when the volume is already attached.
	// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the unit is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNotAlive]: when the storage is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
	// - [github.com/juju/juju/domain/application/errors.InvalidStorageCount]: when the allowed attachment count would be violated.
	// - [github.com/juju/juju/domain/application/errors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
	AttachStorage(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error

	// AddStorageForUnit adds storage instances to given unit as specified.
	// Missing storage constraints are populated based on model defaults.
	// The specified storage name is used to retrieve existing storage instances.
	// Combination of existing storage instances and anticipated additional storage
	// instances is validated as specified in the unit's charm.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the unit is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNotAlive]: when the storage is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
	// - [github.com/juju/juju/domain/application/errors.InvalidStorageCount]: when the allowed attachment count would be violated.
	// - [github.com/juju/juju/domain/application/errors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
	AddStorageForUnit(
		ctx context.Context, storageName corestorage.Name, unitUUID coreunit.UUID, directive storage.Directive,
	) ([]corestorage.ID, error)

	// DetachStorageForUnit detaches the specified storage from the specified unit.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
	DetachStorageForUnit(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error

	// DetachStorage detaches the specified storage from whatever node it is attached to.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
	DetachStorage(ctx context.Context, storageUUID corestorage.UUID) error
}

// StorageProviderValidator is an interface for defining the requirement of an
// external validator that can check assumptions made about storage providers
// when deploying applications.
type StorageProviderValidator interface {
	// CheckPoolSupportsCharmStorage checks that the provided storage
	// pool uuid can be used for provisioning a certain type of charm storage.
	//
	// The following errors may be expected:
	// - [github.com/juju/juju/domain/storage/errors.PoolNotFoundError] when no
	// storage pool exists for the provided pool uuid.
	CheckPoolSupportsCharmStorage(
		context.Context,
		domainstorage.StoragePoolUUID,
		internalcharm.StorageType,
	) (bool, error)

	// CheckProviderTypeSupportsCharmStorage checks that the provider type can
	// be used for provisioning a certain type of charm storage.
	//
	// The following errors may be expected:
	// - [github.com/juju/juju/domain/storage/errors.ProviderTypeNotFound] when
	// no provider type for the supplied name exists.
	CheckProviderTypeSupportsCharmStorage(
		context.Context,
		string,
		internalcharm.StorageType,
	) (bool, error)
}

// AttachStorage attached the specified storage to the specified unit.
// If the attachment already exists, the result is a no op.
// The following error types can be expected:
// - [github.com/juju/juju/core/unit.InvalidUnitName]: when the unit name is not valid.
// - [github.com/juju/juju/core/storage.InvalidStorageID]: when the storage ID is not valid.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
// - [github.com/juju/juju/domain/application/errors.FilesystemAlreadyAttached]: when the filesystem is already attached.
// - [github.com/juju/juju/domain/application/errors.VolumeAlreadyAttached]: when the volume is already attached.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the unit is not alive.
// - [github.com/juju/juju/domain/application/errors.StorageNotAlive]: when the storage is not alive.
// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
// - [github.com/juju/juju/domain/application/errors.InvalidStorageCount]: when the allowed attachment count would be violated.
// - [github.com/juju/juju/domain/application/errors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
func (s *Service) AttachStorage(ctx context.Context, storageID corestorage.ID, unitName coreunit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := storageID.Validate(); err != nil {
		return errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	storageUUID, err := s.st.GetStorageUUIDByID(ctx, storageID)
	if err != nil {
		return errors.Capture(err)
	}
	err = s.st.AttachStorage(ctx, storageUUID, unitUUID)
	if errors.Is(err, applicationerrors.StorageAlreadyAttached) {
		return nil
	}
	return err
}

// AddStorageForUnit adds storage instances to the given unit.
// Missing storage constraints are populated based on model defaults.
// The following error types can be expected:
// - [github.com/juju/juju/core/unit.InvalidUnitName]: when the unit name is not valid.
// - [github.com/juju/juju/core/storage.InvalidStorageName]: when the storage name is not valid.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the unit is not alive.
// - [github.com/juju/juju/domain/application/errors.StorageNotAlive]: when the storage is not alive.
// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
// - [github.com/juju/juju/domain/application/errors.InvalidStorageCount]: when the allowed attachment count would be violated.
// - [github.com/juju/juju/domain/application/errors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
func (s *Service) AddStorageForUnit(
	ctx context.Context, storageName corestorage.Name, unitName coreunit.Name, directive storage.Directive,
) ([]corestorage.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := storageName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return s.st.AddStorageForUnit(ctx, storageName, unitUUID, directive)
}

// DetachStorageForUnit detaches the specified storage from the specified unit.
// The following error types can be expected:
// - [github.com/juju/juju/core/unit.InvalidUnitName]: when the unit name is not valid.
// - [github.com/juju/juju/core/storage.InvalidStorageID]: when the storage ID is not valid.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
func (s *Service) DetachStorageForUnit(ctx context.Context, storageID corestorage.ID, unitName coreunit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := storageID.Validate(); err != nil {
		return errors.Capture(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	storageUUID, err := s.st.GetStorageUUIDByID(ctx, storageID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.st.DetachStorageForUnit(ctx, storageUUID, unitUUID)
}

// DetachStorage detaches the specified storage from whatever node it is attached to.
// The following error types can be expected:
// - [github.com/juju/juju/core/storage.InvalidStorageID]: when the storage ID is not valid.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
func (s *Service) DetachStorage(ctx context.Context, storageID corestorage.ID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := storageID.Validate(); err != nil {
		return errors.Capture(err)
	}
	storageUUID, err := s.st.GetStorageUUIDByID(ctx, storageID)
	if err != nil {
		return errors.Capture(err)
	}
	return s.st.DetachStorage(ctx, storageUUID)
}
