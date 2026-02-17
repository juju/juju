// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"slices"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
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

	logger logger.Logger
}

// State describes retrieval and persistence methods for
// storage related interactions.
type State interface {
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

	// GetApplicationStorageDirectivesInfo returns the storage directives set for an application,
	// keyed to the storage name. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// If the application does not exist, then a [applicationerrors.ApplicationNotFound]
	// error is returned.
	GetApplicationStorageDirectivesInfo(
		ctx context.Context,
		appUUID coreapplication.UUID,
	) (map[string]application.ApplicationStorageInfo, error)

	// GetApplicationStorageDirectives returns the storage directives that are
	// set for an application. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
	// when the application no longer exists.
	GetApplicationStorageDirectives(
		context.Context, coreapplication.UUID,
	) ([]internal.StorageDirective, error)

	// GetModelStoragePools returns the default storage pools
	// that have been set for the model.
	GetModelStoragePools(
		context.Context,
	) (internal.ModelStoragePools, error)

	// GetStorageInstancesForProviderIDs returns all the storage instances
	// found in the model using one of the provider ids supplied. The storage
	// instance must also not be owned by a unit. If no storage instances are
	// found then an empty result is returned.
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

	// GetUnitOwnedStorageInstances returns the storage compositions for all
	// storage instances owned by the unit in the model. If the unit does not
	// currently own any storage instances then an empty result is returned.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	GetUnitOwnedStorageInstances(
		context.Context,
		coreunit.UUID,
	) (
		[]internal.StorageInstanceComposition,
		[]internal.StorageAttachmentComposition,
		error,
	)

	// GetUnitStorageDirectives returns the storage directives that are set for
	// a unit. If the unit does not have any storage directives set then an
	// empty result is returned.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	GetUnitStorageDirectives(
		context.Context, coreunit.UUID,
	) ([]internal.StorageDirective, error)

	// GetUnitStorageDirectiveByName returns the named storage directive that
	// is set for a unit.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	// - [applicationerrors.StorageNameNotSupported] if the named storage directive doesn't exist.
	GetUnitStorageDirectiveByName(
		context.Context, coreunit.UUID, string,
	) (internal.StorageDirective, error)

	// GetUnitNetNodeUUID returns the net node UUID for the specified unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound]: when the unit is not found.
	GetUnitNetNodeUUID(ctx context.Context, uuid coreunit.UUID) (string, error)

	// GetStorageInstanceCompositionByUUID returns the storage compositions for
	// the specified storage instance.
	//
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound]
	// when the storage doesn't exist.
	GetStorageInstanceCompositionByUUID(
		ctx context.Context,
		storageInstanceUUID domainstorage.StorageInstanceUUID,
	) (internal.StorageInstanceComposition, error)
}

// NewService returns a new application storage service for the model.
func NewService(st State, storagePoolProvider StoragePoolProvider, logger logger.Logger) *Service {
	return &Service{
		storagePoolProvider: storagePoolProvider,
		st:                  st,
		logger:              logger,
	}
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
func (s *Service) MakeRegisterExistingCAASUnitStorageArg(
	ctx context.Context,
	unitUUID coreunit.UUID,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
) (domainstorage.RegisterUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	existingUnitStorage, existingUnitStorageAttachments, err := s.st.GetUnitOwnedStorageInstances(
		ctx, unitUUID)
	if err != nil {
		return domainstorage.RegisterUnitStorageArg{}, errors.Errorf(
			"getting unit %q owned storage instances: %w", unitUUID, err,
		)
	}

	directivesToFollow, err := s.st.GetUnitStorageDirectives(ctx, unitUUID)
	if err != nil {
		return domainstorage.RegisterUnitStorageArg{}, errors.Errorf(
			"getting unit %q storage directives: %w", unitUUID, err,
		)
	}

	return s.makeRegisterCAASUnitStorageArg(
		ctx,
		attachmentNetNodeUUID,
		providerFilesystemInfo,
		directivesToFollow,
		existingUnitStorage,
		existingUnitStorageAttachments,
	)
}

// MakeRegisterNewCAASUnitStorageArg is responsible for constructing the storage
// arguments for registering a new caas unit in the model.
//
// The following errors may be expected:
// - [applicationerrors.ApplicationNotFound] when the application no longer
// exists.
func (s *Service) MakeRegisterNewCAASUnitStorageArg(
	ctx context.Context,
	appUUID coreapplication.UUID,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
) (domainstorage.RegisterUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	directivesToFollow, err := s.st.GetApplicationStorageDirectives(
		ctx, appUUID,
	)
	if err != nil {
		return domainstorage.RegisterUnitStorageArg{}, errors.Errorf(
			"getting application %q storage directives: %w", appUUID, err,
		)
	}

	return s.makeRegisterCAASUnitStorageArg(
		ctx,
		attachmentNetNodeUUID,
		providerFilesystemInfo,
		directivesToFollow,
		nil, // new unit so there is no existing storage to supply.
		nil, // new unit so there is also no existing storage attachments.
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
func (s *Service) makeRegisterCAASUnitStorageArg(
	ctx context.Context,
	attachmentNetNodeUUID domainnetwork.NetNodeUUID,
	providerFilesystemInfo []caas.FilesystemInfo,
	directivesToFollow []internal.StorageDirective,
	existingUnitOwnedStorage []internal.StorageInstanceComposition,
	existingUnitOwnedStorageAttachments []internal.StorageAttachmentComposition,
) (domainstorage.RegisterUnitStorageArg, error) {
	storageProviderIDs := make([]string, 0, len(providerFilesystemInfo))
	for _, fsInfo := range providerFilesystemInfo {
		storageProviderIDs = append(storageProviderIDs,
			fsInfo.Volume.PersistentVolumeName)
	}

	// We fetch all existing storage instances in the model that are using one
	// of the provider ids and not owned by a unit.
	existingProviderStorage, err := s.st.GetStorageInstancesForProviderIDs(
		ctx, storageProviderIDs,
	)
	if err != nil {
		return domainstorage.RegisterUnitStorageArg{}, errors.Errorf(
			"getting existing storage instances based on observed provider ids: %w",
			err,
		)
	}

	unitStorageArgs, err := s.MakeUnitStorageArgs(
		ctx,
		attachmentNetNodeUUID,
		directivesToFollow,
		append(existingUnitOwnedStorage, existingProviderStorage...),
		existingUnitOwnedStorageAttachments,
	)
	if err != nil {
		return domainstorage.RegisterUnitStorageArg{}, errors.Errorf(
			"making register caas unit storage args: %w", err,
		)
	}

	// For the existing provider storage instances that are about to be attached
	// make sure they are owned by the unit. Make sure they also have their
	// attachment provider ID mapped if one exists.
	for _, storageInstance := range existingProviderStorage {
		attachmentIndex := slices.IndexFunc(
			unitStorageArgs.StorageToAttach,
			func(e domainstorage.CreateUnitStorageAttachmentArg) bool {
				return e.StorageInstanceUUID == storageInstance.UUID
			},
		)
		if attachmentIndex == -1 {
			continue
		}

		unitStorageArgs.StorageToOwn = append(
			unitStorageArgs.StorageToOwn,
			storageInstance.UUID,
		)
	}

	var (
		filesystemProviderIDs,
		volumeProviderIDs,
		filesystemAttachmentProviderIDs,
		volumeAttachmentProviderIDs = makeCAASStorageInstanceProviderIDAssociations(
			providerFilesystemInfo,
			existingProviderStorage,
			existingUnitOwnedStorage,
			existingUnitOwnedStorageAttachments,
			unitStorageArgs.StorageInstances,
			unitStorageArgs.StorageToAttach,
		)
	)

	return domainstorage.RegisterUnitStorageArg{
		CreateUnitStorageArg:            unitStorageArgs,
		FilesystemProviderIDs:           filesystemProviderIDs,
		VolumeProviderIDs:               volumeProviderIDs,
		FilesystemAttachmentProviderIDs: filesystemAttachmentProviderIDs,
		VolumeAttachmentProviderIDs:     volumeAttachmentProviderIDs,
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
	existingUnitOwnedStorage []internal.StorageInstanceComposition,
	existingUnitAttachments []internal.StorageAttachmentComposition,
	unitStorageToCreate []domainstorage.CreateUnitStorageInstanceArg,
	unitStorageToAttach []domainstorage.CreateUnitStorageAttachmentArg,
) (
	map[domainstorage.FilesystemUUID]string,
	map[domainstorage.VolumeUUID]string,
	map[domainstorage.FilesystemAttachmentUUID]string,
	map[domainstorage.VolumeAttachmentUUID]string,
) {
	rvalFilesystemProviderIDs := map[domainstorage.FilesystemUUID]string{}
	rvalVolumeProviderIDs := map[domainstorage.VolumeUUID]string{}
	rvalFilesystemAttachmentProviderIDs := map[domainstorage.FilesystemAttachmentUUID]string{}
	rvalVolumeAttachmentProviderIDs := map[domainstorage.VolumeAttachmentUUID]string{}

	storageProviderIDsToAttachmentProviderIDs := make(
		map[string]string, len(providerFilesystemInfo),
	)
	for _, fsInfo := range providerFilesystemInfo {
		if fsInfo.PersistentVolumeClaimName == "" {
			continue
		}
		storageProviderIDsToAttachmentProviderIDs[fsInfo.Volume.PersistentVolumeName] = fsInfo.PersistentVolumeClaimName
	}

	unassignedStorageNameToIDMap := map[string][]string{}
	for _, providerFS := range providerFilesystemInfo {
		alreadyInUse := slices.ContainsFunc(
			existingProviderStorage,
			func(e internal.StorageInstanceComposition) bool {
				if e.Filesystem != nil && e.Filesystem.ProviderID == providerFS.Volume.PersistentVolumeName {
					return true
				} else if e.Volume != nil && e.Volume.ProviderID == providerFS.Volume.PersistentVolumeName {
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
			providerFS.Volume.PersistentVolumeName,
		)
	}

	// Assign existing storage instances without a provider ID here.
	for _, inst := range existingUnitOwnedStorage {
		if inst.Filesystem == nil {
			continue
		}
		if inst.Filesystem.ProviderID != "" {
			continue
		}

		storageNameKey := inst.StorageName.String()
		availableIDs, exists := unassignedStorageNameToIDMap[storageNameKey]
		if !exists || len(availableIDs) == 0 {
			// If there is no provider id available for this existing storage
			// instance then we do nothing.
			continue
		}

		rvalFilesystemProviderIDs[inst.Filesystem.UUID] = availableIDs[0]
		unassignedStorageNameToIDMap[storageNameKey] = availableIDs[1:]
	}

	for _, inst := range unitStorageToCreate {
		if inst.Filesystem == nil {
			continue
		}

		storageNameKey := inst.Name.String()
		availableIDs, exists := unassignedStorageNameToIDMap[storageNameKey]
		if !exists || len(availableIDs) == 0 {
			// If there is no provider id available for this new storage
			// instance then we do nothing.
			continue
		}

		rvalFilesystemProviderIDs[inst.Filesystem.UUID] = availableIDs[0]
		unassignedStorageNameToIDMap[storageNameKey] = availableIDs[1:]
	}

filesystemAttachmentLoop:
	for fsUUID, fsProviderID := range rvalFilesystemProviderIDs {
		fsaID, ok := storageProviderIDsToAttachmentProviderIDs[fsProviderID]
		if !ok {
			continue
		}
		for _, v := range existingUnitAttachments {
			if v.FilesystemAttachment == nil {
				continue
			}
			if v.FilesystemAttachment.FilesystemUUID != fsUUID {
				continue
			}
			fsaUUID := v.FilesystemAttachment.UUID
			rvalFilesystemAttachmentProviderIDs[fsaUUID] = fsaID
			continue filesystemAttachmentLoop
		}
		for _, v := range unitStorageToAttach {
			if v.FilesystemAttachment == nil {
				continue
			}
			if v.FilesystemAttachment.FilesystemUUID != fsUUID {
				continue
			}
			fsaUUID := v.FilesystemAttachment.UUID
			rvalFilesystemAttachmentProviderIDs[fsaUUID] = fsaID
			continue filesystemAttachmentLoop
		}
	}

volumeAttachmentLoop:
	for volUUID, volProviderID := range rvalVolumeProviderIDs {
		vaID, ok := storageProviderIDsToAttachmentProviderIDs[volProviderID]
		if !ok {
			continue
		}
		for _, v := range existingUnitAttachments {
			if v.VolumeAttachment == nil {
				continue
			}
			if v.VolumeAttachment.VolumeUUID != volUUID {
				continue
			}
			vaUUID := v.VolumeAttachment.UUID
			rvalVolumeAttachmentProviderIDs[vaUUID] = vaID
			continue volumeAttachmentLoop
		}
		for _, v := range unitStorageToAttach {
			if v.VolumeAttachment == nil {
				continue
			}
			if v.VolumeAttachment.VolumeUUID != volUUID {
				continue
			}
			vaUUID := v.VolumeAttachment.UUID
			rvalVolumeAttachmentProviderIDs[vaUUID] = vaID
			continue volumeAttachmentLoop
		}
	}

	return rvalFilesystemProviderIDs,
		rvalVolumeProviderIDs,
		rvalFilesystemAttachmentProviderIDs,
		rvalVolumeAttachmentProviderIDs
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
) (domainstorage.CreateUnitStorageAttachmentArg, error) {
	uuid, err := domainstorage.NewStorageAttachmentUUID()
	if err != nil {
		return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"generating new storage attachment uuid: %w", err,
		)
	}

	rval := domainstorage.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: storageInstance.UUID,
		UUID:                uuid,
	}

	if storageInstance.Filesystem != nil {
		uuid, err := domainstorage.NewFilesystemAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new filesystem attachment uuid: %w", err,
			)
		}

		rval.FilesystemAttachment = &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: storageInstance.Filesystem.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Filesystem.ProvisionScope,
			UUID:           uuid,
		}
	}

	if storageInstance.Volume != nil {
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new volume attachment uuid: %w", err,
			)
		}

		rval.VolumeAttachment = &domainstorage.CreateUnitStorageVolumeAttachmentArg{
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
	storageInstance domainstorage.CreateUnitStorageInstanceArg,
) (domainstorage.CreateUnitStorageAttachmentArg, error) {
	uuid, err := domainstorage.NewStorageAttachmentUUID()
	if err != nil {
		return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"generating new storage attachment uuid: %w", err,
		)
	}

	rval := domainstorage.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: storageInstance.UUID,
		UUID:                uuid,
	}

	if storageInstance.Filesystem != nil {
		uuid, err := domainstorage.NewFilesystemAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new filesystem attachment uuid: %w", err,
			)
		}

		rval.FilesystemAttachment = &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: storageInstance.Filesystem.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Filesystem.ProvisionScope,
			UUID:           uuid,
		}
	}

	if storageInstance.Volume != nil {
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new volume attachment uuid: %w", err,
			)
		}

		rval.VolumeAttachment = &domainstorage.CreateUnitStorageVolumeAttachmentArg{
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
func (s *Service) MakeUnitStorageArgs(
	ctx context.Context,
	attachNetNodeUUID domainnetwork.NetNodeUUID,
	storageDirectives []internal.StorageDirective,
	existingStorage []internal.StorageInstanceComposition,
	existingStorageAttachments []internal.StorageAttachmentComposition,
) (domainstorage.CreateUnitStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	rvalDirectives := make([]domainstorage.DirectiveArg, 0, len(storageDirectives))
	rvalInstances := []domainstorage.CreateUnitStorageInstanceArg{}
	rvalToAttach := make([]domainstorage.CreateUnitStorageAttachmentArg, 0, len(storageDirectives))
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
		rvalDirectives = append(rvalDirectives, domainstorage.DirectiveArg{
			Count:    sd.Count,
			Name:     sd.Name,
			PoolUUID: sd.PoolUUID,
			Size:     sd.Size,
		})

		existingStorageInstances := existingStorageNameMap[sd.Name.String()]
		maxCount := sd.MaxCount
		if sd.MaxCount == charm.StorageNoMaxCount {
			maxCount = len(existingStorageInstances)
		} else if sd.MaxCount < 0 {
			// This is defensive programming. If by some chance this value is
			// < 0 and not equal to [charm.StorageNoMaxCount] then we will only
			// allow up to the number of existing storage instances. This SHOULD
			// never happen but we have safety rails.
			maxCount = len(existingStorageInstances)
		}

		toUse := min(len(existingStorageInstances), maxCount)
		addCount := sd.Count - min(sd.Count, uint32(toUse)) // We don't want count to underflow.
		instArgs, err := makeUnitStorageInstancesFromDirective(
			ctx,
			addCount,
			storagePoolProvider,
			sd,
		)
		if err != nil {
			return domainstorage.CreateUnitStorageArg{}, errors.Errorf(
				"making new storage %q instance args: %w", sd.Name, err,
			)
		}

		// Allocate capacity we know we are going to need.
		rvalToAttach = slices.Grow(rvalToAttach, len(instArgs)+toUse)
		rvalInstances = slices.Grow(rvalInstances, len(instArgs))
		rvalToOwn = slices.Grow(rvalToOwn, len(instArgs))
		for _, inst := range instArgs {
			storageAttachArg, err := makeStorageAttachmentArgFromNewStorageInstance(
				attachNetNodeUUID, internal.UnitStorageInstanceArg{
					Filesystem: inst.Filesystem,
					Volume:     inst.Volume,
					UUID:       inst.UUID,
				},
			)

			if err != nil {
				return domainstorage.CreateUnitStorageArg{}, errors.Errorf(
					"making storage attachment arguments for new storage instance: %w", err,
				)
			}

			rvalToOwn = append(rvalToOwn, inst.UUID)
			rvalToAttach = append(rvalToAttach, storageAttachArg)
			rvalInstances = append(rvalInstances, inst)
		}

		existingStorageToUse := existingStorageInstances[:toUse]
	storageToAttachLoop:
		for _, inst := range existingStorageToUse {
			for _, existingAttachment := range existingStorageAttachments {
				if existingAttachment.StorageInstanceUUID == inst.UUID {
					// This storage instance is already attached to this unit.
					continue storageToAttachLoop
				}
			}
			storageAttachArg, err :=
				makeStorageAttachmentArgFromExistingStorageInstance(
					attachNetNodeUUID, inst,
				)
			if err != nil {
				return domainstorage.CreateUnitStorageArg{}, errors.Errorf(
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

	return domainstorage.CreateUnitStorageArg{
		StorageDirectives: rvalDirectives,
		StorageInstances:  rvalInstances,
		StorageToAttach:   rvalToAttach,
		StorageToOwn:      rvalToOwn,
	}, nil
}

// MakeIAASUnitStorageArgs returns [domainstorage.CreateIAASUnitStorageArg] that
// complement the unit storage arguments provided for IAAS units.
func (s *Service) MakeIAASUnitStorageArgs(
	ctx context.Context,
	storageInst []domainstorage.CreateUnitStorageInstanceArg,
) (domainstorage.CreateIAASUnitStorageArg, error) {
	var arg domainstorage.CreateIAASUnitStorageArg
	for _, v := range storageInst {
		// TODO(storage): refactor this to use the storage instance composition
		// calculated from the storageprovisioning domain.
		var comp domainstorageprov.StorageInstanceComposition
		if v.Filesystem != nil {
			comp.FilesystemRequired = true
			comp.FilesystemProvisionScope = v.Filesystem.ProvisionScope
		}
		if v.Volume != nil {
			comp.VolumeRequired = true
			comp.VolumeProvisionScope = v.Volume.ProvisionScope
		}
		s, err := domainstorageprov.CalculateStorageInstanceOwnershipScope(
			comp)
		if err != nil {
			return domainstorage.CreateIAASUnitStorageArg{}, errors.Errorf(
				"calculating storage ownership for storage instance %q: %w",
				v.UUID, err,
			)
		}
		if s != domainstorageprov.OwnershipScopeMachine {
			continue
		}
		if v.Filesystem != nil {
			arg.FilesystemsToOwn = append(arg.FilesystemsToOwn,
				v.Filesystem.UUID)
		}
		if v.Volume != nil {
			arg.VolumesToOwn = append(arg.VolumesToOwn, v.Volume.UUID)
		}
	}
	return arg, nil
}

// MakeUnitAddStorageArgs creates the storage arguments required to
// add storage to a unit. This is similar to [MakeUnitStorageArgs]
// but without processing existing storage.
// The details of the new instances are calculated and all the
// required storage attachments are added.
// The directive provides storage defaults including count, but here the
// caller is specifying the actual count to use.
// This is a cut down version of [MakeUnitStorageArgs]. We may
// choose to DRY things up a bit later.
func (s *Service) MakeUnitAddStorageArgs(
	ctx context.Context,
	unitUUID coreunit.UUID,
	addCount uint32,
	sd internal.StorageDirective,
) (domainstorage.UnitAddStorageArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	var rvalInstances []domainstorage.CreateUnitStorageInstanceArg
	rvalToAttach := make([]domainstorage.CreateUnitStorageAttachmentArg, 0, 1)
	// rvalToOwn is the list of storage instance UUIDs that the unit must own.
	rvalToOwn := make([]domainstorage.StorageInstanceUUID, 0, 1)

	// We create a cached storage pool provider for the scope of this operation.
	// This exists to reduce load on the controller potentially requesting the
	// same storage pool provider over and over again.
	storagePoolProvider := cachedStoragePoolProvider{
		Cache:               map[domainstorage.StoragePoolUUID]storage.Provider{},
		StoragePoolProvider: s.storagePoolProvider,
	}

	instArgs, err := makeUnitStorageInstancesFromDirective(
		ctx,
		addCount,
		storagePoolProvider,
		sd,
	)
	if err != nil {
		return domainstorage.UnitAddStorageArg{}, errors.Errorf(
			"making new storage %q instance args: %w", sd.Name, err,
		)
	}

	attachNetNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return domainstorage.UnitAddStorageArg{}, errors.Errorf("getting unit net node uuid: %w", err)
	}

	// Allocate capacity we know we are going to need.
	rvalToAttach = slices.Grow(rvalToAttach, len(instArgs))
	rvalInstances = slices.Grow(rvalInstances, len(instArgs))
	rvalToOwn = slices.Grow(rvalToOwn, len(instArgs))
	for _, inst := range instArgs {
		storageAttachArg, err := makeStorageAttachmentArgFromNewStorageInstance(
			domainnetwork.NetNodeUUID(attachNetNodeUUID), internal.UnitStorageInstanceArg{
				Filesystem: inst.Filesystem,
				Volume:     inst.Volume,
				UUID:       inst.UUID,
			},
		)

		if err != nil {
			return domainstorage.UnitAddStorageArg{}, errors.Errorf(
				"making storage attachment arguments for new storage instance: %w", err,
			)
		}

		rvalToOwn = append(rvalToOwn, inst.UUID)
		rvalToAttach = append(rvalToAttach, storageAttachArg)
		rvalInstances = append(rvalInstances, inst)
	}

	return domainstorage.UnitAddStorageArg{
		StorageInstances: rvalInstances,
		StorageToAttach:  rvalToAttach,
		StorageToOwn:     rvalToOwn,
	}, nil
}

// makeUnitStorageInstancesFromDirective is responsible for taking a storage
// directive and creating a set of storage instance args that are capable of
// fulfilling the requirements of the directive.
// The directive provides storage defaults including count, but here the
// caller is specifying the actual count to use.
func makeUnitStorageInstancesFromDirective(
	ctx context.Context,
	count uint32,
	storagePoolProvider StoragePoolProvider,
	directive internal.StorageDirective,
) ([]domainstorage.CreateUnitStorageInstanceArg, error) {
	// Early exit if no storage instances are to be created. Save's a lot of
	// busy work that goes unused.
	if count == 0 {
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

	rval := make([]domainstorage.CreateUnitStorageInstanceArg, 0, count)
	for range count {
		uuid, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Errorf(
				"new storage instance uuid: %w", err,
			)
		}

		instArg := domainstorage.CreateUnitStorageInstanceArg{
			CharmName:       directive.CharmMetadataName,
			Kind:            storageKind,
			Name:            directive.Name,
			RequestSizeMiB:  directive.Size,
			StoragePoolUUID: directive.PoolUUID,
			UUID:            uuid,
		}

		if composition.FilesystemRequired {
			u, err := domainstorage.NewFilesystemUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage filesystem uuid: %w", err,
				)
			}

			instArg.Filesystem = &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           u,
				ProvisionScope: composition.FilesystemProvisionScope,
			}
		}

		if composition.VolumeRequired {
			u, err := domainstorage.NewVolumeUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage volume uuid: %w", err,
				)
			}

			instArg.Volume = &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           u,
				ProvisionScope: composition.VolumeProvisionScope,
			}
		}

		rval = append(rval, instArg)
	}

	return rval, nil
}

// MakeUnitAttachStorageArgs creates the storage arguments required to
// attach existing storage to a unit.
func (s Service) MakeUnitAttachStorageArgs(
	ctx context.Context,
	unitUUID coreunit.UUID,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) (internal.CreateUnitStorageAttachmentArg, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instComposition, err := s.st.GetStorageInstanceCompositionByUUID(ctx, storageInstanceUUID)
	if err != nil {
		return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"getting composition details for storage %q: %w", storageInstanceUUID, err,
		)
	}

	attachNetNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf("getting unit net node uuid: %w", err)
	}

	instArg := internal.UnitStorageInstanceArg{
		UUID: storageInstanceUUID,
	}
	if instComposition.Filesystem != nil {
		instArg.Filesystem = &internal.CreateUnitStorageFilesystemArg{
			UUID:           instComposition.Filesystem.UUID,
			ProvisionScope: instComposition.Filesystem.ProvisionScope,
		}
	}
	if instComposition.Volume != nil {
		instArg.Volume = &internal.CreateUnitStorageVolumeArg{
			UUID:           instComposition.Volume.UUID,
			ProvisionScope: instComposition.Volume.ProvisionScope,
		}
	}
	storageAttachArg, err := makeStorageAttachmentArgFromNewStorageInstance(
		domainnetwork.NetNodeUUID(attachNetNodeUUID), instArg,
	)
	if err != nil {
		return internal.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"making storage attachment arguments for new storage instance: %w", err,
		)
	}
	return storageAttachArg, nil
}
