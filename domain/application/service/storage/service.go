// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"slices"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// ProviderState defines the required interface of the model's state for
// interacting with storage providers.
type ProviderState interface {
	// GetProviderTypeForPool returns the provider type that is in use for the
	// given pool.
	//
	// The following error types can be expected:
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	GetProviderTypeForPool(context.Context, domainstorage.StoragePoolUUID) (string, error)
}

// Service defines an internal service to this package that groups and
// establishes storage related operations for applications in the model.
type Service struct {
	st State

	storagePoolProvider StoragePoolProvider
}

// State describes retrieval and persistence methods for
// storage related interactions.
type State interface {
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
	AttachStorage(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID) error

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
	DetachStorageForUnit(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID) error

	// DetachStorage detaches the specified storage from whatever node it is attached to.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
	DetachStorage(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID) error

	// GetApplicationStorage returns the storage directives set for an application,
	// keyed to the storage name. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// If the application does not exist, then a [applicationerrors.ApplicationNotFound]
	// error is returned.
	GetApplicationStorage(
		ctx context.Context,
		appUUID coreapplication.UUID,
	) (application.ApplicationStorage, error)

	// GetApplicationStorageDirectives returns the storage directives that are
	// set for an application. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
	// when the application no longer exists.
	GetApplicationStorageDirectives(
		context.Context, coreapplication.UUID,
	) ([]application.StorageDirective, error)

	// GetModelStoragePools returns the default storage pools
	// that have been set for the model.
	GetModelStoragePools(
		context.Context,
	) (internal.ModelStoragePools, error)

	// GetStorageInstancesForProviderIDs returns all of the storage instances
	// found in the model using one of the provider ids supplied. The storage
	// instance must also not be owned by a unit. If no storage instances are found
	// then an empty result is returned.
	GetStorageInstancesForProviderIDs(
		ctx context.Context,
		ids []string,
	) ([]internal.StorageInstanceComposition, error)

	// GetStorageUUIDByID returns the UUID for the storage specified by id.
	//
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if the
	// storage doesn't exist.
	GetStorageUUIDByID(
		ctx context.Context, storageID corestorage.ID,
	) (domainstorage.StorageInstanceUUID, error)

	// GetUnitOwnedStorageInstances returns the storage instance compositions
	// for all storage instances owned by the unit in the model. If the unit
	// does not currently own any storage instances then an empty result is
	// returned.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	GetUnitOwnedStorageInstances(
		context.Context,
		coreunit.UUID,
	) ([]internal.StorageInstanceComposition, error)

	// GetUnitStorageDirectives returns the storage directives that are set for
	// a unit. If the unit does not have any storage directives set then an
	// empty result is returned.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	GetUnitStorageDirectives(
		context.Context, coreunit.UUID,
	) ([]application.StorageDirective, error)
}

// NewService returns a new application storage service for the model.
func NewService(st State, storagePoolProvider StoragePoolProvider) *Service {
	return &Service{
		storagePoolProvider: storagePoolProvider,
		st:                  st,
	}
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
func (s *Service) AttachStorage(
	ctx context.Context, storageID corestorage.ID, unitName coreunit.Name,
) error {
	// TODO (tlm): re-implement in DQlite
	return errors.New("not implemented")
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
	// TODO (tlm): re-implement in DQlite
	return nil, errors.New("not implemented")
}

// encodeStorageKindFromCharmStorageType provides a mapping from charm storage
// type to storage kind.
func encodeStorageKindFromCharmStorageType(
	storageType charm.StorageType,
) (domainstorage.StorageKind, error) {
	switch storageType {
	case charm.StorageBlock:
		return domainstorage.StorageKindBlock, nil
	case charm.StorageFilesystem:
		return domainstorage.StorageKindFilesystem, nil
	default:
		return -1, errors.Errorf(
			"no mapping exists from charm storage type %q to storage kind",
			storageType,
		)
	}
}

// MakeRegisterExistingCAASUnitStorageArg is responsible for constructing the
// storage arguments for registering an existing caas unit in the model. This
// ends up being a set of arguments that are making sure eventual consistency
// of the unit's storage.MakeRegisterExistingCAASUnitStorageArg
//
// The following errors may be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
func (s Service) MakeRegisterExistingCAASUnitStorageArg(
	ctx context.Context,
	unitUUID coreunit.UUID,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
) (internal.RegisterUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	existingUnitStorage, err := s.st.GetUnitOwnedStorageInstances(ctx, unitUUID)
	if err != nil {
		return internal.RegisterUnitStorageArg{}, errors.Errorf(
			"getting unit %q owned storage instances: %w", unitUUID, err,
		)
	}

	directivesToFollow, err := s.st.GetUnitStorageDirectives(ctx, unitUUID)
	if err != nil {
		return internal.RegisterUnitStorageArg{}, errors.Errorf(
			"getting unit %q storage directives: %w", unitUUID, err,
		)
	}

	return s.makeRegisterCAASUnitStorageArg(
		ctx,
		attachmentNetNodeUUID,
		providerFilesystemInfo,
		directivesToFollow,
		existingUnitStorage,
	)
}

// MakeRegisterNewCAASUnitStorageArg is responsible for constructing the storage
// arguments for registering a new caas unit in the model.
//
// The following errors may be expected:
// - [applicationerrors.ApplicationNotFound] when the application no longer
// exists.
func (s Service) MakeRegisterNewCAASUnitStorageArg(
	ctx context.Context,
	appUUID coreapplication.UUID,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
) (internal.RegisterUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	directivesToFollow, err := s.st.GetApplicationStorageDirectives(
		ctx, appUUID,
	)
	if err != nil {
		return internal.RegisterUnitStorageArg{}, errors.Errorf(
			"getting application %q storage directives: %w", appUUID, err,
		)
	}

	return s.makeRegisterCAASUnitStorageArg(
		ctx,
		attachmentNetNodeUUID,
		providerFilesystemInfo,
		directivesToFollow,
		nil, // new unit so there is no existing storage to supply.
	)
}

// makeRegisterCAASUnitStorageArg is responsible for making the storage
// arguments required to register a CAAS unit in the model. This func considers
// pre existing storage already in the model for the unit and any new storage
// that needs to be created.
//
// This function will first use all the existing storage in the model for the
// unit before creating new storage to meet the storage directives of the unit.
// Storage created by this func will be associated with the providers
// information on first creation. All storage created and re-used will also now
// be owned by the unit being registered.
//
// The following errors may be expected:
// - [applicationerrors.ApplicationNotFound] when the application no longer
// exists.
func (s Service) makeRegisterCAASUnitStorageArg(
	ctx context.Context,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
	directivesToFollow []application.StorageDirective,
	existingUnitOwnedStorage []internal.StorageInstanceComposition,
) (internal.RegisterUnitStorageArg, error) {
	// We don't consider the volume information in the caas filesystem info.
	providerIDs := make([]string, 0, len(providerFilesystemInfo))
	for _, fsInfo := range providerFilesystemInfo {
		providerIDs = append(providerIDs, fsInfo.FilesystemId)
	}

	// We fetch all existing storage instances in the model that are using one
	// of the provider ids and not owned by a unit.
	existingProviderStorage, err := s.st.GetStorageInstancesForProviderIDs(
		ctx, providerIDs,
	)
	if err != nil {
		return internal.RegisterUnitStorageArg{}, errors.Errorf(
			"getting existing storage instances based on observed provider ids: %w",
			err,
		)
	}

	unitStorageArgs, err := s.MakeUnitStorageArgs(
		ctx,
		attachmentNetNodeUUID,
		directivesToFollow,
		append(existingUnitOwnedStorage, existingProviderStorage...),
	)
	if err != nil {
		return internal.RegisterUnitStorageArg{}, errors.Errorf(
			"making register caas unit storage args: %w", err,
		)
	}

	// For the existing provider storage instances that are about to be attached
	// make sure they are owned by the unit.
	for _, storageInstance := range existingProviderStorage {
		isBeingAttached := slices.ContainsFunc(
			unitStorageArgs.StorageToAttach,
			func(e internal.CreateUnitStorageAttachmentArg) bool {
				return e.StorageInstanceUUID == storageInstance.UUID
			},
		)
		if !isBeingAttached {
			continue
		}

		unitStorageArgs.StorageToOwn = append(
			unitStorageArgs.StorageToOwn,
			storageInstance.UUID,
		)
	}

	filesystemProviderIDs, volumeProviderIDs :=
		makeCAASStorageInstanceProviderIDAssociations(
			providerFilesystemInfo,
			existingProviderStorage,
			unitStorageArgs.StorageInstances,
		)

	return internal.RegisterUnitStorageArg{
		CreateUnitStorageArg:  unitStorageArgs,
		FilesystemProviderIDs: filesystemProviderIDs,
		VolumeProviderIDs:     volumeProviderIDs,
	}, nil
}

// DetachStorageForUnit detaches the specified storage from the specified unit.
// The following error types can be expected:
// - [github.com/juju/juju/core/unit.InvalidUnitName]: when the unit name is not valid.
// - [github.com/juju/juju/core/storage.InvalidStorageID]: when the storage ID is not valid.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] when the storage doesn't exist.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the unit does not exist.
// - [github.com/juju/juju/domain/application/errors.StorageNotDetachable]: when the type of storage is not detachable.
func (s *Service) DetachStorageForUnit(
	ctx context.Context, storageID corestorage.ID, unitName coreunit.Name,
) error {
	// TODO (tlm): re-implement in DQlite
	return errors.New("not implemented")
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

// makeCAASStorageInstanceProviderIDAssociations takes the reported filesystem
// information from a CAAS unit and associates the reported provider ids to new
// storage instances that are to be created for the unit.
//
// This function will not use any provider ids that are already associated with
// a storage instance in the existing provider storage supplied.
//
// No reconciliation is done to ensure that each new unit storage has an
// assigned provider id or that all provider ids are consumed.
func makeCAASStorageInstanceProviderIDAssociations(
	providerFilesystemInfo []caas.FilesystemInfo,
	existingProviderStorage []internal.StorageInstanceComposition,
	unitStorageToCreate []internal.CreateUnitStorageInstanceArg,
) (
	map[domainstorageprov.FilesystemUUID]string,
	map[domainstorageprov.VolumeUUID]string,
) {
	rvalFilesystemProviderIDs := map[domainstorageprov.FilesystemUUID]string{}
	rvalVolumeProviderIDs := map[domainstorageprov.VolumeUUID]string{}

	unassignedStorageNameToIDMap := map[string][]string{}
	for _, providerFS := range providerFilesystemInfo {
		alreadyInUse := slices.ContainsFunc(
			existingProviderStorage,
			func(e internal.StorageInstanceComposition) bool {
				if e.Filesystem != nil && e.Filesystem.ProviderID == providerFS.FilesystemId {
					return true
				} else if e.Volume != nil && e.Volume.ProviderID == providerFS.FilesystemId {
					return true
				}
				return false
			},
		)
		if alreadyInUse {
			continue
		}

		unassignedStorageNameToIDMap[providerFS.StorageName] = append(
			unassignedStorageNameToIDMap[providerFS.StorageName],
			providerFS.FilesystemId,
		)
	}

	for _, inst := range unitStorageToCreate {
		availableIDs, exists := unassignedStorageNameToIDMap[inst.Name.String()]
		// If there is not provider id available for this new storage instance
		// then we do nothing.
		if !exists || len(availableIDs) == 0 {
			continue
		}

		if inst.Filesystem != nil {
			rvalFilesystemProviderIDs[inst.Filesystem.UUID] = availableIDs[0]
		}
		if inst.Volume != nil {
			rvalVolumeProviderIDs[inst.Volume.UUID] = availableIDs[0]
		}
	}

	return rvalFilesystemProviderIDs, rvalVolumeProviderIDs
}

// makeStorageAttachmentArgFromExistingStorageInstance is responsible for taking
// an existing storage instance in the model and generating a corresponding
// storage attachment creation argument.
//
// The attachment of the filesystem and volume will be done on to the supplied
// net node and follow the information set on the existing storage instance.
func makeStorageAttachmentArgFromExistingStorageInstance(
	netNodeUUID domainnetwork.NetNodeUUID,
	storageInstance internal.StorageInstanceComposition,
) (internal.CreateUnitStorageAttachmentArg, error) {
	uuid, err := domainstorageprov.NewStorageAttachmentUUID()
	if err != nil {
		return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"generating new storage attachment uuid: %w", err,
		)
	}

	rval := internal.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: storageInstance.UUID,
		UUID:                uuid,
	}

	if storageInstance.Filesystem != nil {
		uuid, err := domainstorageprov.NewFilesystemAttachmentUUID()
		if err != nil {
			return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new filesystem attachment uuid: %w", err,
			)
		}

		rval.FilesystemAttachment = &internal.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: storageInstance.Filesystem.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Filesystem.ProvisionScope,
			UUID:           uuid,
		}
	}

	if storageInstance.Volume != nil {
		uuid, err := domainstorageprov.NewVolumeAttachmentUUID()
		if err != nil {
			return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new volume attachment uuid: %w", err,
			)
		}

		rval.VolumeAttachment = &internal.CreateUnitStorageVolumeAttachmentArg{
			VolumeUUID:     storageInstance.Volume.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Volume.ProvisionScope,
			UUID:           uuid,
		}
	}

	return rval, nil
}

// makeStorageAttachmentArgFromNewStorageInstance is responsible for taking the
// arguments to create a new storage instance in the model and generating a
// corresponding storage attachment creation argument.
//
// The attachment of filesystem and volume will be done on to the supplied net
// node and follow the information set on the storage instance.
func makeStorageAttachmentArgFromNewStorageInstance(
	netNodeUUID domainnetwork.NetNodeUUID,
	storageInstance internal.CreateUnitStorageInstanceArg,
) (internal.CreateUnitStorageAttachmentArg, error) {
	uuid, err := domainstorageprov.NewStorageAttachmentUUID()
	if err != nil {
		return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"generating new storage attachment uuid: %w", err,
		)
	}

	rval := internal.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: storageInstance.UUID,
		UUID:                uuid,
	}

	if storageInstance.Filesystem != nil {
		uuid, err := domainstorageprov.NewFilesystemAttachmentUUID()
		if err != nil {
			return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new filesystem attachment uuid: %w", err,
			)
		}

		rval.FilesystemAttachment = &internal.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: storageInstance.Filesystem.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Filesystem.ProvisionScope,
			UUID:           uuid,
		}
	}

	if storageInstance.Volume != nil {
		uuid, err := domainstorageprov.NewVolumeAttachmentUUID()
		if err != nil {
			return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new volume attachment uuid: %w", err,
			)
		}

		rval.VolumeAttachment = &internal.CreateUnitStorageVolumeAttachmentArg{
			VolumeUUID:     storageInstance.Volume.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Volume.ProvisionScope,
			UUID:           uuid,
		}
	}

	return rval, nil
}

// MakeUnitStorageArgs creates the storage arguments required for a unit in
// the model. This func looks at the set of directives for the unit and the
// existing storage available. From this any new instances that need to be
// created are calculated and all storage attachments are added.
//
// The attach netnode uuid argument tell this func what enitities are being
// attached to in the model.
//
// Existing storage supplied to this function will not be included in the
// storage ownership of the unit. It is expected the unit owns or will own this
// storage.
//
// No guarantee is made that existing storage supplied to this func will be used
// in it's entirety. If a storage directive has less demand then what is
// supplied it is possible that some existing storage will be unused. It is up
// to the caller to validate what storage was and wasn't used by looking at the
// storage attachments.
func (s Service) MakeUnitStorageArgs(
	ctx context.Context,
	attachNetNodeUUID domainnetwork.NetNodeUUID,
	storageDirectives []application.StorageDirective,
	existingStorage []internal.StorageInstanceComposition,
) (internal.CreateUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	rvalDirectives := make([]internal.CreateUnitStorageDirectiveArg, 0, len(storageDirectives))
	rvalInstances := []internal.CreateUnitStorageInstanceArg{}
	rvalToAttach := make([]internal.CreateUnitStorageAttachmentArg, 0, len(storageDirectives))
	// rvalToOwn is the list of storage instance uuid's that the unit must own.
	rvalToOwn := make([]domainstorage.StorageInstanceUUID, 0, len(storageDirectives))

	// We create a cahced storage pool provider for the scope of this operation.
	// This exists to reduce load on the controller potentially requesting the
	// same storage pool provider over and over again.
	storagePoolProvider := cachedStoragePoolProvider{
		Cache:               map[domainstorage.StoragePoolUUID]storage.Provider{},
		StoragePoolProvider: s.storagePoolProvider,
	}

	existingStorageNameMap := map[string][]internal.StorageInstanceComposition{}
	for _, es := range existingStorage {
		existingStorageNameMap[es.StorageName.String()] = append(
			existingStorageNameMap[es.StorageName.String()], es,
		)
	}

	for _, sd := range storageDirectives {
		// Make the storage directive arg first. This MUST happen as the count
		// value in [sd] is about to be modified.
		rvalDirectives = append(rvalDirectives, internal.CreateUnitStorageDirectiveArg{
			Count:    sd.Count,
			Name:     sd.Name,
			PoolUUID: sd.PoolUUID,
			Size:     sd.Size,
		})

		existingStorageInstances := existingStorageNameMap[sd.Name.String()]
		toUse := min(uint32(len(existingStorageInstances)), sd.MaxCount)
		sd.Count -= min(sd.Count, toUse)

		instArgs, err := makeUnitStorageInstancesFromDirective(
			ctx,
			storagePoolProvider,
			sd,
		)
		if err != nil {
			return internal.CreateUnitStorageArg{}, errors.Errorf(
				"making new storage %q instance args: %w", sd.Name, err,
			)
		}

		// Allocate capacity we know we are going to need.
		rvalToAttach = slices.Grow(rvalToAttach, len(instArgs)+int(toUse))
		rvalInstances = slices.Grow(rvalInstances, len(instArgs))
		rvalToOwn = slices.Grow(rvalToOwn, len(instArgs))
		for _, inst := range instArgs {
			storageAttachArg, err := makeStorageAttachmentArgFromNewStorageInstance(
				attachNetNodeUUID, inst,
			)

			if err != nil {
				return internal.CreateUnitStorageArg{}, errors.Errorf(
					"making storage attachment arguments for new storage instance: %w", err,
				)
			}

			rvalToOwn = append(rvalToOwn, inst.UUID)
			rvalToAttach = append(rvalToAttach, storageAttachArg)
			rvalInstances = append(rvalInstances, inst)
		}

		existingStorageToUse := existingStorageInstances[:toUse]
		for _, inst := range existingStorageToUse {
			storageAttachArg, err :=
				makeStorageAttachmentArgFromExistingStorageInstance(
					attachNetNodeUUID, inst,
				)
			if err != nil {
				return internal.CreateUnitStorageArg{}, errors.Errorf(
					"making storage attachment argument for existing storage instance %q: %w",
					inst.UUID, err,
				)
			}
			rvalToAttach = append(rvalToAttach, storageAttachArg)
		}

		// Remove the storage instances that we have used from the map.
		existingStorageNameMap[sd.Name.String()] =
			existingStorageInstances[toUse:]
	}

	return internal.CreateUnitStorageArg{
		StorageDirectives: rvalDirectives,
		StorageInstances:  rvalInstances,
		StorageToAttach:   rvalToAttach,
		StorageToOwn:      rvalToOwn,
	}, nil
}

// makeUnitStorageInstancesFromDirective is responsible for taking a storage
// directive and creating a set of storage instance args that are capable of
// fulfilling the requirements of the directive.
func makeUnitStorageInstancesFromDirective(
	ctx context.Context,
	storagePoolProvider StoragePoolProvider,
	directive application.StorageDirective,
) ([]internal.CreateUnitStorageInstanceArg, error) {
	// Early exit if no storage instances are to be created. Save's a lot of
	// busy work that goes unused.
	if directive.Count == 0 {
		return nil, nil
	}

	storageKind, err := encodeStorageKindFromCharmStorageType(directive.CharmStorageType)
	if err != nil {
		return nil, errors.Capture(err)
	}

	provider, err := storagePoolProvider.GetProviderForPool(
		ctx, directive.PoolUUID,
	)
	if err != nil {
		return nil, errors.Errorf(
			"getting storage provider for storage directive pool %q: %w",
			directive.PoolUUID, err,
		)
	}

	composition, err := domainstorageprov.CalculateStorageInstanceComposition(
		storageKind, provider,
	)
	if err != nil {
		return nil, errors.Errorf(
			"calculating storage entity composition for directive: %w", err,
		)
	}

	rval := make([]internal.CreateUnitStorageInstanceArg, 0, directive.Count)
	for range directive.Count {
		uuid, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Errorf(
				"new storage instance uuid: %w", err,
			)
		}

		instArg := internal.CreateUnitStorageInstanceArg{
			CharmName:       directive.CharmMetadataName,
			Kind:            storageKind,
			Name:            directive.Name,
			RequestSizeMiB:  directive.Size,
			StoragePoolUUID: directive.PoolUUID,
			UUID:            uuid,
		}

		if composition.FilesystemRequired {
			u, err := domainstorageprov.NewFilesystemUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage filesystem uuid: %w", err,
				)
			}

			instArg.Filesystem = &internal.CreateUnitStorageFilesystemArg{
				UUID:           u,
				ProvisionScope: composition.FilesystemProvisionScope,
			}
		}

		if composition.VolumeRequired {
			u, err := domainstorageprov.NewVolumeUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage volume uuid: %w", err,
				)
			}

			instArg.Volume = &internal.CreateUnitStorageVolumeArg{
				UUID:           u,
				ProvisionScope: composition.VolumeProvisionScope,
			}
		}

		rval = append(rval, instArg)
	}

	return rval, nil
}
