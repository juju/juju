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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// DefaultStorageProviderValidator is the default implementation of
// [StorageProviderValidator] for this domain.
type DefaultStorageProviderValidator struct {
	providerRegistryGetter corestorage.ModelStorageRegistryGetter
	st                     StorageProviderState
}

// StorageProviderState defines the required interface of the model's state for
// interacting with storage providers.
type StorageProviderState interface {
	// GetProviderTypeOfPool returns the provider type that is in use for the
	// given pool.
	//
	// The following error types can be expected:
	// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
	// provided pool uuid.
	GetProviderTypeOfPool(context.Context, domainstorage.StoragePoolUUID) (string, error)
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

// StorageProviderValidator is an interface for defining the requirement of an
// external validator that can check assumptions made about storage providers
// when deploying applications.
type StorageProviderValidator interface {
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
func (v *DefaultStorageProviderValidator) CheckPoolSupportsCharmStorage(
	ctx context.Context,
	poolUUID domainstorage.StoragePoolUUID,
	storageType internalcharm.StorageType,
) (bool, error) {
	if err := poolUUID.Validate(); err != nil {
		return false, errors.Errorf(
			"storage pool uuid is not valid: %w", err,
		).Add(coreerrors.NotValid)
	}

	providerTypeStr, err := v.st.GetProviderTypeOfPool(ctx, poolUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	providerRegistry, err := v.providerRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return false, errors.Errorf(
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
		return false, errors.Errorf(
			"provider type %q for storage pool %q does not exist",
			providerTypeStr, poolUUID,
		).Add(storageerrors.ProviderTypeNotFound)
	} else if err != nil {
		return false, errors.Errorf(
			"getting storage provider for pool %q: %w", poolUUID, err,
		)
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

// makeUnitStorageArgs creates the storage arguments required for a CAAS unit in
// the model. This func looks at the set of directives for the unit and the
// existing storage available. From this any new instances that need to be
// created are calculated and all storage attachments are added.
func makeUnitStorageArgs(
	storageDirectives []application.StorageDirective,
	existingStorage map[domainstorage.Name][]domainstorage.StorageInstanceUUID,
) (application.CreateUnitStorageArg, error) {
	rvalDirectives := make([]application.CreateUnitStorageDirectiveArg, 0, len(storageDirectives))
	rvalInstances := []application.CreateUnitStorageInstanceArg{}
	rvalToAttach := make([]domainstorage.StorageInstanceUUID, 0, len(storageDirectives))
	rvalToOwn := make([]domainstorage.StorageInstanceUUID, 0, len(storageDirectives))

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
		sd.Count -= numExistingStorage

		// Add the existing storage matching this directive to the list of
		// attachments.
		rvalToAttach = append(rvalToAttach, existingStorageUUIDs...)
		rvalToOwn = append(rvalToOwn, existingStorageUUIDs...)

		instArgs, err := makeUnitStorageInstancesFromDirective(sd)
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
		rvalToAttach = append(rvalToAttach, inst.UUID)
		rvalToOwn = append(rvalToOwn, inst.UUID)
	}

	return application.CreateUnitStorageArg{
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
	directive application.StorageDirective,
) ([]application.CreateUnitStorageInstanceArg, error) {
	rval := make([]application.CreateUnitStorageInstanceArg, 0, directive.Count)
	for range directive.Count {
		uuid, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Errorf(
				"new storage instance uuid: %w", err,
			)
		}

		fsUUID, err := domainstorageprov.NewFileystemUUID()
		if err != nil {
			return nil, errors.Errorf(
				"generating new storage filesystem uuid: %w", err,
			)
		}
		// TODO (tlm): We are only focused on Kubernetes storage working at the
		// moment. For that reason we are just hardcoding out volume and
		// filesystem. This will be updated to cover all storage and for a
		// machine in the near future.
		rval = append(rval, application.CreateUnitStorageInstanceArg{
			Name:           directive.Name,
			UUID:           uuid,
			FilesystemUUID: &fsUUID,
		})
	}

	return rval, nil
}

// makeStorageDirectiveFromApplicationArg is responsible take the storage
// directive create params for an application and converting them into
// [application.StorageDirective] types for creating units.
func makeStorageDirectiveFromApplicationArg(
	applicationArgs []application.CreateApplicationStorageDirectiveArg,
) []application.StorageDirective {
	rval := make([]application.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, application.StorageDirective{
			Name:     arg.Name,
			Count:    arg.Count,
			PoolUUID: arg.PoolUUID,
			Size:     arg.Size,
		})
	}

	return rval
}

// NewStorageProviderValidator returns a new [DefaultStorageProviderValidator]
// that allows checking of storage providers against expected storage
// requirements.
func NewStorageProviderValidator(
	providerRegistryGetter corestorage.ModelStorageRegistryGetter,
	st StorageProviderState,
) *DefaultStorageProviderValidator {
	return &DefaultStorageProviderValidator{
		providerRegistryGetter: providerRegistryGetter,
		st:                     st,
	}
}
