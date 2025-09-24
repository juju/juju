// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// cachedStoragePoolProvider is a special implementation of
// [StoragePoolProvider] it exists to provide a temporary read through cache of
// storage providers used by a storage pool.
//
// For example if the provider is asked to provide the provider for a storage
// pool it will cache the provider so that future questions of the same pool can
// return the provider in the cache.
//
// This type exists to be short lived. It should only ever be created for single
// operation that requires fetching a storage pools provide multiple times in
// the operation.
//
// This implementation is NOT thread safe and never will be. Short operations
// with a defined end that ask the same question repeatedly is that this type
// exists to solve.
type cachedStoragePoolProvider struct {
	// StoragePoolProvider is the storage pool provider that is wrapped by this
	// cache.
	StoragePoolProvider

	// Cache is the internal cache used. This value must be initialised by the
	// user.
	Cache map[domainstorage.StoragePoolUUID]storage.Provider
}

// StoragePoolProvider defines an interface by where provider based questions
// for storage pools can be asked. This interface acts as the bridge between a
// storage pool and the underlying provider that is used.
type StoragePoolProvider interface {
	// CheckPoolSupportsCharmStorage checks that the provided storage
	// pool uuid can be used for provisioning a certain type of charm storage.
	//
	// The following errors may be expected:
	// - [coreerrors.NotValid] if the provided pool uuid is not valid.
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	CheckPoolSupportsCharmStorage(
		context.Context,
		domainstorage.StoragePoolUUID,
		internalcharm.StorageType,
	) (bool, error)

	// GetProviderForPool returns the storage provider that is backing a given
	// storage pool. This is a utility func for this domain to enable asking
	// questions of a provider when you are starting with a storage pool.
	//
	// The following errors may be expected:
	// - [coreerrors.NotValid] if the provided pool uuid is not valid.
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	GetProviderForPool(
		context.Context, domainstorage.StoragePoolUUID,
	) (storage.Provider, error)
}

// DefaultStoragePoolProvider is the default implementation of
// [StoragePoolProvider] for this domain.
type DefaultStoragePoolProvider struct {
	providerRegistryGetter corestorage.ModelStorageRegistryGetter
	st                     StorageProviderState
}

// StorageProviderState defines the required interface of the model's state for
// interacting with storage providers.
type StorageProviderState interface {
	// GetProviderTypeForPool returns the provider type that is in use for the
	// given pool.
	//
	// The following error types can be expected:
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	GetProviderTypeForPool(context.Context, domainstorage.StoragePoolUUID) (string, error)
}

// StorageState describes retrieval and persistence methods for
// storage related interactions.
type StorageState interface {

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

	// GetApplicationStorageDirectives returns the storage directives that are
	// set for an application. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
	// when the application no longer exists.
	GetApplicationStorageDirectives(
		context.Context, coreapplication.ID,
	) ([]application.StorageDirective, error)

	// GetDefaultStorageProvisioners returns the default storage provisioners
	// that have been set for the model.
	GetDefaultStorageProvisioners(
		ctx context.Context,
	) (application.DefaultStorageProvisioners, error)

	// GetStorageInstancesForProviderIDs returns the storage instance uuid
	// associated with a provider id. Only storage instances belonging to an
	// application are considered.
	//
	// If a storage instance doesn't exist for a provider id then this is not an
	// error and no data will be emitted for the id. There is no correlation
	// between ids supplied and instances supplied.
	//
	// The caller should expect that a zero length result can be supplied.
	GetStorageInstancesForProviderIDs(
		ctx context.Context,
		applicationUUID coreapplication.ID,
		ids []string,
	) (map[string]domainstorage.StorageInstanceUUID, error)

	// GetStorageUUIDByID returns the UUID for the storage specified by id.
	//
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if the
	// storage doesn't exist.
	GetStorageUUIDByID(
		ctx context.Context, storageID corestorage.ID,
	) (domainstorage.StorageInstanceUUID, error)

	// GetUnitOwnedStorageInstances returns the storage instances that are owned
	// by a unit. If the unit does not currently own any storage instances an
	// empty result is returned.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit no longer exists.
	GetUnitOwnedStorageInstances(
		context.Context,
		coreunit.UUID,
	) (map[domainstorage.Name][]domainstorage.StorageInstanceUUID, error)

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

// NewStoragePoolProvider returns a new [DefaultStoragePoolProvider]
// that allows getting provider information for a storage pool.
//
// The returned [DefaultStoragePoolProvider] implements the
// [StoragePoolProvider] interface.
func NewStoragePoolProvider(
	providerRegistryGetter corestorage.ModelStorageRegistryGetter,
	st StorageProviderState,
) *DefaultStoragePoolProvider {
	return &DefaultStoragePoolProvider{
		providerRegistryGetter: providerRegistryGetter,
		st:                     st,
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

// CheckPoolSupportsCharmStorage checks that the provided storage
// pool uuid can be used for provisioning a certain type of charm storage.
//
// The following errors may be expected:
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (v *DefaultStoragePoolProvider) CheckPoolSupportsCharmStorage(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
	storageType internalcharm.StorageType,
) (bool, error) {
	provider, err := v.GetProviderForPool(ctx, poolUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	switch storageType {
	case internalcharm.StorageFilesystem:
		return provider.Supports(storage.StorageKindFilesystem), nil
	case internalcharm.StorageBlock:
		return provider.Supports(storage.StorageKindBlock), nil
	default:
		return false, errors.Errorf(
			"unknown charm storage type %q", storageType,
		)
	}
}

// GetProviderForPool returns the storage provider that is backing a given
// storage pool. This is a utility func for this domain to enable asking
// questions of a provider when you are starting with a storage pool.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (v *DefaultStoragePoolProvider) GetProviderForPool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (storage.Provider, error) {
	if err := poolUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"storage pool uuid is not valid: %w", err,
		).Add(coreerrors.NotValid)
	}

	providerTypeStr, err := v.st.GetProviderTypeForPool(ctx, poolUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	providerRegistry, err := v.providerRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model storage provider registry: %w", err,
		)
	}

	providerType := storage.ProviderType(providerTypeStr)
	provider, err := providerRegistry.StorageProvider(providerType)
	// We check if the error is for the provider type not being found and
	// translate it over to a ProviderTypeNotFound error. This error type is not
	// recorded in the contract as  this should never be possible. But we are
	// being a good citizen and returning meaningful errors.
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf(
			"provider type %q for storage pool %q does not exist",
			providerTypeStr, poolUUID,
		).Add(storageerrors.ProviderTypeNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage provider for pool %q: %w", poolUUID, err,
		)
	}

	return provider, nil
}

// GetProviderForPool returns the storage provider associated with the given
// storage pool. This func will first consult the cache to see if the provider
// is available there and then if not proxy the call through to the underlying
// [StorageProviderPool].
//
// This func is not thread safe and never will be. Implements the
// [StorageProviderPool] interface.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the provided pool uuid is not valid.
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (c cachedStoragePoolProvider) GetProviderForPool(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
) (storage.Provider, error) {
	provider, has := c.Cache[poolUUID]
	if has {
		return provider, nil
	}

	provider, err := c.StoragePoolProvider.GetProviderForPool(ctx, poolUUID)
	if err != nil {
		return nil, err
	}

	c.Cache[poolUUID] = provider
	return provider, nil
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

// getUnitStorageInfo retrieves the existing storage information for a unit.
// This includes the storage directives set for the unit and also any storage
// that is currently owned by the unit.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
func (s *ProviderService) getUnitStorageInfo(
	ctx context.Context,
	unitUUID coreunit.UUID,
) (
	[]application.StorageDirective,
	map[domainstorage.Name][]domainstorage.StorageInstanceUUID,
	error,
) {
	existingUnitStorage, err := s.st.GetUnitOwnedStorageInstances(
		ctx, unitUUID,
	)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting unit %q owned storage instances: %w",
			unitUUID, err,
		)
	}

	storageDirectives, err := s.st.GetUnitStorageDirectives(ctx, unitUUID)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting unit %q storage directives: %w",
			unitUUID, err,
		)
	}

	return storageDirectives, existingUnitStorage, nil
}

// makeUnitStorageArgs creates the storage arguments required for a unit in
// the model. This func looks at the set of directives for the unit and the
// existing storage available. From this any new instances that need to be
// created are calculated and all storage attachments are added.
func makeUnitStorageArgs(
	ctx context.Context,
	storagePoolProvider StoragePoolProvider,
	storageDirectives []application.StorageDirective,
	existingStorage map[domainstorage.Name][]domainstorage.StorageInstanceUUID,
) (application.CreateUnitStorageArg, error) {
	rvalDirectives := make([]application.CreateUnitStorageDirectiveArg, 0, len(storageDirectives))
	rvalInstances := []application.CreateUnitStorageInstanceArg{}
	rvalToAttach := make([]application.CreateStorageAttachmentArg, 0, len(storageDirectives))
	// rvalToOwn is the list of storage instance uuid's that the unit must own.
	rvalToOwn := make([]domainstorage.StorageInstanceUUID, 0, len(storageDirectives))

	storagePoolProvider = cachedStoragePoolProvider{
		Cache:               map[domainstorage.StoragePoolUUID]storage.Provider{},
		StoragePoolProvider: storagePoolProvider,
	}

	for _, sd := range storageDirectives {
		// We make the storage directive args first. This is becase we know we
		// will change the count in the struct later.
		rvalDirectives = append(rvalDirectives, application.CreateUnitStorageDirectiveArg{
			Count:    sd.Count,
			Name:     sd.Name,
			PoolUUID: sd.PoolUUID,
			Size:     sd.Size,
		})

		existingStorageUUIDs := existingStorage[sd.Name]
		if len(existingStorageUUIDs) > math.MaxUint32 {
			return application.CreateUnitStorageArg{}, errors.Errorf(
				"storage %q has too many storage instances", sd.Name,
			)
		}
		numExistingStorage := uint32(len(existingStorageUUIDs))
		if numExistingStorage > sd.Count {
			// A storage directive only supports n number of storage instances
			// per directive. If there exists more existing storage for this
			// directive than supported, we return an error.
			//
			// This is undefined behaviour in the Juju modeling.
			return application.CreateUnitStorageArg{}, errors.Errorf(
				"unable to use %d existing storage instances for directive %q, greater than supported count %d",
				numExistingStorage, sd.Name, sd.Count,
			)
		}

		// Remove the already existing storage instances from the count.
		// The remainder being what needs to be created.
		sd.Count -= numExistingStorage

		rvalToOwn = append(rvalToOwn, existingStorageUUIDs...)

		instArgs, err := makeUnitStorageInstancesFromDirective(
			ctx,
			storagePoolProvider,
			sd,
		)
		if err != nil {
			return application.CreateUnitStorageArg{}, errors.Errorf(
				"making new storage instance args: %w", err,
			)
		}

		rvalInstances = append(rvalInstances, instArgs...)
	}

	// For all the new storage instances that need to be created add their uuids
	// to the set of attachments.
	for _, inst := range rvalInstances {
		rvalToOwn = append(rvalToOwn, inst.UUID)
	}
	saArgs, err := makeStorageAttachmentArgs(rvalToOwn)
	if err != nil {
		return application.CreateUnitStorageArg{}, errors.Errorf(
			"making new storage attachment args: %w", err,
		)
	}
	rvalToAttach = append(rvalToAttach, saArgs...)

	return application.CreateUnitStorageArg{
		StorageDirectives: rvalDirectives,
		StorageInstances:  rvalInstances,
		StorageToAttach:   rvalToAttach,
		StorageToOwn:      rvalToOwn,
	}, nil
}

func makeStorageAttachmentArgs(
	storageInstanceUUIDs []domainstorage.StorageInstanceUUID,
) ([]application.CreateStorageAttachmentArg, error) {
	var attachments []application.CreateStorageAttachmentArg
	for _, inst := range storageInstanceUUIDs {
		saUUID, err := domainstorageprov.NewStorageAttachmentUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		attachments = append(attachments, application.CreateStorageAttachmentArg{
			UUID:                saUUID,
			StorageInstanceUUID: inst,
		})
	}
	return attachments, nil
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

// makeUnitStorageInstancesFromDirective is responsible for taking a storage
// directive and creating a set of storage instance args that are capable of
// fulfilling the requirements of the directive.
func makeUnitStorageInstancesFromDirective(
	ctx context.Context,
	storagePoolProvider StoragePoolProvider,
	directive application.StorageDirective,
) ([]application.CreateUnitStorageInstanceArg, error) {
	// Early exit if no storage instances are to be created. Save's a lot of
	// busy work that goes unused.
	if directive.Count == 0 {
		return nil, nil
	}

	storageKind, err := encodeStorageKindFromCharmStorageType(directive.Type)
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

	rval := make([]application.CreateUnitStorageInstanceArg, 0, directive.Count)
	for range directive.Count {
		uuid, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Errorf(
				"new storage instance uuid: %w", err,
			)
		}

		instArg := application.CreateUnitStorageInstanceArg{
			Kind: storageKind,
			Name: directive.Name,
			UUID: uuid,
		}

		if composition.FilesystemRequired {
			u, err := domainstorageprov.NewFilesystemUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage filesystem uuid: %w", err,
				)
			}

			instArg.Filesystem = &application.CreateUnitStorageFilesystemArg{
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

			instArg.Volume = &application.CreateUnitStorageVolumeArg{
				UUID:           u,
				ProvisionScope: composition.VolumeProvisionScope,
			}
		}

		rval = append(rval, instArg)
	}

	return rval, nil
}

// makeStorageDirectiveFromApplicationArg is responsible take the storage
// directive create params for an application and converting them into
// [application.StorageDirective] types for creating units.
func makeStorageDirectiveFromApplicationArg(
	charmStorage map[string]internalcharm.Storage,
	applicationArgs []application.CreateApplicationStorageDirectiveArg,
) []application.StorageDirective {
	rval := make([]application.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, application.StorageDirective{
			Name:     arg.Name,
			Count:    arg.Count,
			Type:     charm.StorageType(charmStorage[arg.Name.String()].Type),
			PoolUUID: arg.PoolUUID,
			Size:     arg.Size,
		})
	}

	return rval
}
