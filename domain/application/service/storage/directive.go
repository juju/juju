// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// StorageDirectiveOverride defines a single set of overrides for a
// storage directive to accompany an application when it is being added.
// Typically, this is supplied by the caller when the user whishes to set
// explicitly storage directive parameters of the application.
//
// Each value in this struct is optional and only when a value that is non nil
// has been supplied will it be used to override the defaults.
type StorageDirectiveOverride struct {
	// Count is the number of storage instances to create for each unit. This
	// value must be greater or equal to the minimum defined by the charm. This
	// value must also be less or equal to the maximum defined by the charm.
	Count *uint32

	// PoolUUID defines the storage pool to use when provisioning storage for
	// this directive.
	PoolUUID *domainstorage.StoragePoolUUID

	// Size defines the size of the storage to provision as a minimum value in
	// MiB. What gets provisioned by the provider for each unit may be larger
	// then this value.
	Size *uint64
}

const (
	// defaultStorageDirectiveSize is the default size used for application
	// storage directives when no other size value can be used. This value
	// is in MiB.
	defaultStorageDirectiveSize = 1024 // 1 GiB
)

// GetApplicationStorageDirectives returns the storage directives that are
// set for an application. If the application does not have any storage
// directives set then an empty result is returned.
//
// The following error types can be expected:
// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
// when the application no longer exists.
func (s *Service) GetApplicationStorageDirectives(
	ctx context.Context,
	uuid coreapplication.UUID,
) ([]application.StorageDirective, error) {
	if uuid.Validate() != nil {
		return nil, errors.New("application uuid is not valid").Add(coreerrors.NotValid)
	}
	return s.st.GetApplicationStorageDirectives(ctx, uuid)
}

// GetUnitStorageDirectiveByName returns the named storage directive for the unit.
// The following errors may be expected:
// - [coreerrors.NotValid] when the supplied unit uuid is not valid.
// - [applicationerrors.StorageNameNotSupported] if the named storage does not exist.
func (s *Service) GetUnitStorageDirectiveByName(
	ctx context.Context,
	uuid coreunit.UUID,
	storageName corestorage.Name,
) (application.StorageDirective, error) {
	if uuid.Validate() != nil {
		return application.StorageDirective{}, errors.New("unit uuid is not valid").Add(coreerrors.NotValid)
	}
	if err := storageName.Validate(); err != nil {
		return application.StorageDirective{}, errors.Capture(err)
	}

	return s.st.GetUnitStorageDirectiveByName(ctx, uuid, storageName.String())
}

// GetApplicationStorageDirectivesInfo returns the storage directives set for an application,
// keyed to the storage name. If the application does not have any storage
// directives set then an empty result is returned.
//
// The following errors may be expected:
// - [coreerrors.NotValid] when the supplied application uuid is not valid.
// - [applicationerrors.ApplicationNotFound] if the application does not exist.
func (s *Service) GetApplicationStorageDirectivesInfo(
	ctx context.Context,
	uuid coreapplication.UUID,
) (map[string]application.ApplicationStorageInfo, error) {
	if uuid.Validate() != nil {
		return nil, errors.New("application uuid is not valid").Add(coreerrors.NotValid)
	}
	return s.st.GetApplicationStorageDirectivesInfo(ctx, uuid)
}

// MakeApplicationStorageDirectiveArgs creates a slice of
// [application.CreateApplicationStorageDirectiveArg] from a set of overrides
// and the charm storage information. The resultant directives are a merging of
// all the data sources to form an approximation of what the storage directives
// for an application should be.
//
// The directives SHOULD still be validated.
func (s *Service) MakeApplicationStorageDirectiveArgs(
	ctx context.Context,
	directiveOverrides map[string]StorageDirectiveOverride,
	charmMetaStorage map[string]internalcharm.Storage,
) ([]internal.CreateApplicationStorageDirectiveArg, error) {
	if len(charmMetaStorage) == 0 {
		return nil, nil
	}

	modelStoragePools, err := s.st.GetModelStoragePools(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting default storage provisioners for model: %w", err,
		)
	}

	rval := make([]internal.CreateApplicationStorageDirectiveArg, 0, len(charmMetaStorage))
	for charmStorageName, charmStorageDef := range charmMetaStorage {
		// We don't support shared storage. If the charm has a shared storage
		// definition we ignore it.
		if charmStorageDef.Shared {
			continue
		}

		arg := makeApplicationStorageDirectiveArg(
			domainstorage.Name(charmStorageName),
			directiveOverrides[charmStorageName],
			charmStorageDef,
			modelStoragePools,
		)
		rval = append(rval, arg)
	}
	return rval, nil
}

// makeApplicationStorageDirectiveArg creates a
// [application.ApplicationStorageDirectiveArgs] based on the overrides supplied
// by the caller, the information contained within the charm and the default
// provisioners supplied.
//
// The resultant directive argument is not garuranteed to be valid and should
// still be checked. This function just offers a merge of all the information
// sources to create the best approximation at an application storage directive.
func makeApplicationStorageDirectiveArg(
	name domainstorage.Name,
	directiveOverride StorageDirectiveOverride,
	charmStorageDef internalcharm.Storage,
	modelStoragePools internal.ModelStoragePools,
) internal.CreateApplicationStorageDirectiveArg {
	rval := internal.CreateApplicationStorageDirectiveArg{
		Name: name,
	}

	rval.Count = 0
	// If the charm storage definition has a negative min count we maintain zero
	// as the directive value. Defensive programming against a bad cast that
	// would produce an incorrect value for an already incorrect value.
	if charmStorageDef.CountMin > 0 {
		rval.Count = uint32(charmStorageDef.CountMin)
	}
	if directiveOverride.Count != nil {
		rval.Count = *directiveOverride.Count
	}

	rval.Size = defaultStorageDirectiveSize
	if charmStorageDef.MinimumSize > 0 {
		rval.Size = charmStorageDef.MinimumSize
	}
	if directiveOverride.Size != nil {
		rval.Size = *directiveOverride.Size
	}

	if directiveOverride.PoolUUID != nil {
		// Set the pool uuid to the value supplied by the override.
		rval.PoolUUID = *directiveOverride.PoolUUID
	} else if modelStoragePools.BlockDevicePoolUUID != nil &&
		charmStorageDef.Type == internalcharm.StorageBlock {
		// Set the pool uuid if the charm storage is block and a block pool
		// provisioner exists.
		rval.PoolUUID = *modelStoragePools.BlockDevicePoolUUID
	} else if modelStoragePools.FilesystemPoolUUID != nil &&
		charmStorageDef.Type == internalcharm.StorageFilesystem {
		// Set the pool uuid if the charm storage is filesystem and a filesystem
		// pool provisioner exists.
		rval.PoolUUID = *modelStoragePools.FilesystemPoolUUID
	}

	return rval
}

// MakeStorageDirectiveFromApplicationArg is responsible for takeing the storage
// directive create params for an application and converting them into
// [application.StorageDirective] types.
func MakeStorageDirectiveFromApplicationArg(
	charmMetadataName string,
	charmStorage map[string]internalcharm.Storage,
	applicationArgs []internal.CreateApplicationStorageDirectiveArg,
) []application.StorageDirective {
	rval := make([]application.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, application.StorageDirective{
			CharmMetadataName: charmMetadataName,
			Count:             arg.Count,
			CharmStorageType:  charm.StorageType(charmStorage[arg.Name.String()].Type),
			MaxCount:          charmStorage[arg.Name.String()].CountMax,
			Name:              arg.Name,
			PoolUUID:          arg.PoolUUID,
			Size:              arg.Size,
		})
	}

	return rval
}

// ValidateApplicationStorageDirectiveOverrides checks a set of storage
// directive overrides to make sure they are valid with respect to the charms
// storage definitions.
//
// The following errors may be expected:
// - [applicationerrors.StorageCountLimitExceeded] when the requested storage
// falls outside of the bounds defined by the charm.
func (s *Service) ValidateApplicationStorageDirectiveOverrides(
	ctx context.Context,
	charmStorageDefs map[string]internalcharm.Storage,
	overrides map[string]StorageDirectiveOverride,
) error {
	for name, override := range overrides {
		storageDef, exists := charmStorageDefs[name]
		if !exists {
			return errors.Errorf(
				"storage directive %q does not exist in the charm", name,
			)
		}

		err := validateApplicationStorageDirectiveOverride(
			ctx, storageDef, override, s.storagePoolProvider,
		)
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

// validateApplicationStorageDirectiveOverride checks a set of storage directive
// override values to make sure they are valid with respect to the charm
// storage.
//
// The following errors may be expected:
// - [applicationerrors.StorageCountLimitExceeded] the the requested storage
// falls outside of the bounds defined by the charm.
func validateApplicationStorageDirectiveOverride(
	ctx context.Context,
	charmStorageDef internalcharm.Storage,
	override StorageDirectiveOverride,
	poolProvider StoragePoolProvider,
) error {
	if override.Count != nil {
		var minCount uint32
		if charmStorageDef.CountMin > 0 {
			minCount = uint32(charmStorageDef.CountMin)
		}

		var (
			// hasMaxCount is true when the charm storage definition has
			// indicated that there is a maximum value it will tolerate. When
			// the charm specifies -1 the charm has no opinion what the maximum
			// should be.
			hasMaxCount bool
			maxCount    uint32
		)
		if charmStorageDef.CountMax >= 0 {
			maxCount = uint32(charmStorageDef.CountMax)
			hasMaxCount = true
		}

		if *override.Count < minCount {
			return applicationerrors.StorageCountLimitExceeded{
				Minimum:     charmStorageDef.CountMin,
				Requested:   int(*override.Count),
				StorageName: charmStorageDef.Name,
			}
		}
		if hasMaxCount && *override.Count > maxCount {
			return applicationerrors.StorageCountLimitExceeded{
				Maximum:     &charmStorageDef.CountMax,
				Minimum:     charmStorageDef.CountMin,
				Requested:   int(*override.Count),
				StorageName: charmStorageDef.Name,
			}
		}
	}

	// If the override has changed storage and the charm storage definition
	// expects a minimum size (not zero), then we check that override size is
	// less then the minimum size.
	if override.Size != nil &&
		charmStorageDef.MinimumSize != 0 &&
		*override.Size < charmStorageDef.MinimumSize {
		return errors.Errorf(
			"storage directive size %d is less than the charm minimum requirement of %d",
			*override.Size, charmStorageDef.MinimumSize,
		)
	}

	if override.PoolUUID != nil {
		charmStorageType := charm.StorageType(charmStorageDef.Type)
		supports, err := poolProvider.CheckPoolSupportsCharmStorage(
			ctx, *override.PoolUUID, charmStorageType,
		)
		if err != nil {
			return errors.Errorf(
				"checking storage directive pool %q supports charm storage %q",
				*override.PoolUUID, charmStorageDef.Type,
			)
		}

		if !supports {
			return errors.Errorf(
				"storage directive pool %q does not support charm storage %q",
				*override.PoolUUID, charmStorageDef.Type,
			)
		}
	}

	return nil
}

// ReconcileUpdatedCharmStorageDirective merges existing application storage directives with
// new charm storage requirements.
func ReconcileUpdatedCharmStorageDirective(
	newCharmStorages map[string]internalcharm.Storage,
	existingStorageDirectives []application.StorageDirective,
	modelStoragePools internal.ModelStoragePools,
) (
	toApply []internal.ApplyApplicationStorageDirectiveArg,
	toDelete []string,
	err error,
) {
	toApply = []internal.ApplyApplicationStorageDirectiveArg{}
	toDelete = []string{}

	// To check later on which new storage should be created.
	processedNewCharmStorage := make(map[string]bool)

	existingStorageDirectivesMap := transform.SliceToMap(existingStorageDirectives, func(d application.StorageDirective) (string, application.StorageDirective) {
		return d.Name.String(), d
	})

	// Process existing directives against new charm storage.
	// We process only overlapping storage names here for update.
	for storageName, existingStorageDirective := range existingStorageDirectivesMap {
		newCharmStorage, existsInCharm := newCharmStorages[storageName]

		// Storage no longer in charm, mark for deletion.
		if !existsInCharm {
			toDelete = append(toDelete, storageName)
			continue
		}

		// We return an error if the storage type has changed for the same storage name,
		// preserving the behaviour and err message we had in 3.6.
		if existingStorageDirective.CharmStorageType.String() != newCharmStorage.Type.String() {
			return nil, nil, &applicationerrors.CharmStorageTypeChanged{
				StorageName: storageName,
				OldType:     existingStorageDirective.CharmStorageType.String(),
				NewType:     newCharmStorage.Type.String(),
			}
		}

		// Reconcile this directive.
		reconciledArg := reconcileStorageDirective(existingStorageDirective, storageName, newCharmStorage)

		// Only include in toApply if something changed.
		if hasStorageDirectiveChanged(existingStorageDirective, reconciledArg) {
			toApply = append(toApply, reconciledArg)
		}

		processedNewCharmStorage[storageName] = true
	}

	// Process creating new charm storage that are not in existing directives.
	for charmStorageName, newCharmStorage := range newCharmStorages {
		if processedNewCharmStorage[charmStorageName] {
			continue
		}

		arg := createApplyApplicationStorageDirectiveArg(
			charmStorageName,
			newCharmStorage,
			modelStoragePools,
		)
		toApply = append(toApply, arg)
	}

	return toApply, toDelete, nil
}

// reconcileStorageDirective reconciles an existing directive with new charm requirements.
func reconcileStorageDirective(
	existingStorageDirective application.StorageDirective,
	storageName string,
	newCharmStorage internalcharm.Storage,
) internal.ApplyApplicationStorageDirectiveArg {
	arg := internal.ApplyApplicationStorageDirectiveArg{
		Name: domainstorage.Name(storageName),
	}

	// Increase count if below new minimum.
	minCount := uint32(max(newCharmStorage.CountMin, 0))
	count := max(minCount, existingStorageDirective.Count)

	// If the charm has a max count defined (!= -1) and the max count is greater
	// then the existing storage directive count decrease the storage directive
	// count to match.
	maxCount := uint32(newCharmStorage.CountMax)
	if newCharmStorage.CountMax >= 0 && count > maxCount {
		count = maxCount
	}

	// Reconcile storage count.
	arg.Count = count

	// Reconcile storage size.
	arg.Size = max(existingStorageDirective.Size, newCharmStorage.MinimumSize)

	// Preserve the existing pool UUID.
	arg.PoolUUID = existingStorageDirective.PoolUUID

	return arg
}

// hasStorageDirectiveChanged checks if a directive needs updating.
func hasStorageDirectiveChanged(
	existing application.StorageDirective,
	proposed internal.ApplyApplicationStorageDirectiveArg,
) bool {
	storageCountChanged := proposed.Count != existing.Count
	storageSizeChanged := proposed.Size != existing.Size
	storagePoolChanged := proposed.PoolUUID != existing.PoolUUID

	return storageCountChanged || storageSizeChanged || storagePoolChanged
}

// createApplyApplicationStorageDirectiveArg creates a directive for new storage in the charm.
// This is intended to be used for charm refresh.
func createApplyApplicationStorageDirectiveArg(
	storageName string,
	charmStorage internalcharm.Storage,
	modelStoragePools internal.ModelStoragePools,
) internal.ApplyApplicationStorageDirectiveArg {

	arg := internal.ApplyApplicationStorageDirectiveArg{
		Name: domainstorage.Name(storageName),
	}
	// Set count.
	if charmStorage.CountMin > 0 {
		arg.Count = uint32(charmStorage.CountMin)
	}

	// Set size.
	arg.Size = defaultStorageDirectiveSize
	if charmStorage.MinimumSize > 0 {
		arg.Size = charmStorage.MinimumSize
	}

	// Set poolUUID.
	if charmStorage.Type == internalcharm.StorageBlock && modelStoragePools.BlockDevicePoolUUID != nil {
		arg.PoolUUID = *modelStoragePools.BlockDevicePoolUUID
	} else if charmStorage.Type == internalcharm.StorageFilesystem && modelStoragePools.FilesystemPoolUUID != nil {
		arg.PoolUUID = *modelStoragePools.FilesystemPoolUUID
	}
	return arg
}
