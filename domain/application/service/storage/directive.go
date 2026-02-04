// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"math"

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

const (
	// defaultStorageDirectiveSize is the default size used for application
	// storage directives when no other size value can be used. This value
	// is in MiB.
	defaultStorageDirectiveSize = 1024 // 1 GiB
)

// StorageDirectiveOverride is a type alias for the domain type.
type StorageDirectiveOverride = application.ApplicationStorageDirectiveOverride

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
) ([]internal.StorageDirective, error) {
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
) (internal.StorageDirective, error) {
	if uuid.Validate() != nil {
		return internal.StorageDirective{}, errors.New("unit uuid is not valid").Add(coreerrors.NotValid)
	}
	if err := storageName.Validate(); err != nil {
		return internal.StorageDirective{}, errors.Capture(err)
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
// [domainstorage.DirectiveArg] from a set of overrides
// and the charm storage information. The resultant directives are a merging of
// all the data sources to form an approximation of what the storage directives
// for an application should be.
//
// The directives SHOULD still be validated.
func (s *Service) MakeApplicationStorageDirectiveArgs(
	ctx context.Context,
	directiveOverrides map[string]StorageDirectiveOverride,
	charmMetaStorage map[string]internalcharm.Storage,
) ([]domainstorage.DirectiveArg, error) {
	if len(charmMetaStorage) == 0 {
		return nil, nil
	}

	modelStoragePools, err := s.st.GetModelStoragePools(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting default storage provisioners for model: %w", err,
		)
	}

	rval := make([]domainstorage.DirectiveArg, 0, len(charmMetaStorage))
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
// [domainstorage.DirectiveArg] based on the overrides supplied
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
) domainstorage.DirectiveArg {
	rval := domainstorage.DirectiveArg{
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
	applicationArgs []domainstorage.DirectiveArg,
) []internal.StorageDirective {
	rval := make([]internal.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, internal.StorageDirective{
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
	charmStorageDefs map[string]internal.ValidateStorageArg,
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
	charmStorageDef internal.ValidateStorageArg,
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

// ValidateAttachStorage checks that a storage instance from the specified
// pool can be attached to a unit with respect to the unit's charm storage
// definition.
func (s *Service) ValidateAttachStorage(
	ctx context.Context,
	charmStorageDef internal.ValidateStorageArg,
	wantCount uint32,
	storageSize uint64,
	poolUUID domainstorage.StoragePoolUUID,
) error {
	return validateAttachStorage(ctx, charmStorageDef, wantCount, storageSize, poolUUID, s.storagePoolProvider)
}

func validateAttachStorage(
	ctx context.Context,
	charmStorageDef internal.ValidateStorageArg,
	wantCount uint32,
	storageSize uint64,
	poolUUID domainstorage.StoragePoolUUID,
	poolProvider StoragePoolProvider,
) error {
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

	if hasMaxCount && wantCount > maxCount {
		return applicationerrors.StorageCountLimitExceeded{
			Maximum:     &charmStorageDef.CountMax,
			Minimum:     charmStorageDef.CountMin,
			Requested:   int(wantCount),
			StorageName: charmStorageDef.Name,
		}
	}

	if charmStorageDef.MinimumSize != 0 &&
		storageSize < charmStorageDef.MinimumSize {
		return errors.Errorf(
			"storage directive size %d is less than the charm minimum requirement of %d",
			storageSize, charmStorageDef.MinimumSize,
		)
	}

	charmStorageType := charm.StorageType(charmStorageDef.Type)
	supports, err := poolProvider.CheckPoolSupportsCharmStorage(
		ctx, poolUUID, charmStorageType,
	)
	if err != nil {
		return errors.Errorf(
			"checking storage directive pool %q supports charm storage %q",
			poolUUID, charmStorageDef.Type,
		)
	}

	if !supports {
		return errors.Errorf(
			"storage directive pool %q does not support charm storage %q",
			poolUUID, charmStorageDef.Type,
		)
	}

	return nil
}

// ReconcileStorageDirectivesAgainstCharmStorage reconciles existing application storage directives
// and adds any new storage definitions.
func (s *Service) ReconcileStorageDirectivesAgainstCharmStorage(
	ctx context.Context,
	existingStorageDirectives []internal.StorageDirective,
	newCharmStorages map[string]internalcharm.Storage,
) (
	toCreate []domainstorage.DirectiveArg,
	toUpdate []domainstorage.DirectiveArg,
	err error,
) {
	// To check later on which new storage should be created.
	processedNewCharmStorage := make(map[string]bool)

	// Reconcile storage names that already exist on the application.
	for _, existingStorageDirective := range existingStorageDirectives {
		storageName := existingStorageDirective.Name.String()
		newCharmStorage, exists := newCharmStorages[storageName]
		// This should not happen.
		// Removal of storage definition in the charm should be blocked in charm compatibility check before this point.
		if !exists {
			return nil, nil, errors.Errorf("existing storage directive %q does not exist in the new charm", storageName)
		}

		// Reconcile after compatibility validation succeeds.
		// This ensures that min size, min count and max count changes in the existing storage directive are
		// reconciled with the new charm storage requirements.
		reconciledArg := ReconcileStorageDirective(existingStorageDirective, newCharmStorage)

		// Include in toUpdate regardless of any value change. We have to update charmUUID regardless.
		toUpdate = append(toUpdate, reconciledArg)
		processedNewCharmStorage[storageName] = true
	}
	modelStoragePools, err := s.st.GetModelStoragePools(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting model storage pools: %w", err)
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
		toCreate = append(toCreate, arg)
	}

	return toCreate, toUpdate, nil
}

// ReconcileStorageDirective reconciles an existing directive with new charm requirements.
func ReconcileStorageDirective(
	existingStorageDirective internal.StorageDirective,
	newCharmStorage internalcharm.Storage,
) domainstorage.DirectiveArg {
	arg := domainstorage.DirectiveArg{
		Name: existingStorageDirective.Name,
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
	// We do not change the storage directive count if the count
	// is already within the new min and max count defined by the new charm.
	arg.Count = count

	// Reconcile storage size.
	// We do not decrease storage directive size if the new charm storage has a smaller minimum size.
	arg.Size = max(existingStorageDirective.Size, newCharmStorage.MinimumSize)

	// Preserve the existing pool UUID.
	arg.PoolUUID = existingStorageDirective.PoolUUID

	return arg
}

// createApplyApplicationStorageDirectiveArg creates a directive for new storage in the charm.
// This is intended to be used for charm refresh.
func createApplyApplicationStorageDirectiveArg(
	storageName string,
	charmStorage internalcharm.Storage,
	modelStoragePools internal.ModelStoragePools,
) domainstorage.DirectiveArg {

	arg := domainstorage.DirectiveArg{
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

// ValidateApplicationStorageDirectives performs a sanity check on the
// directives for the application to make sure they are in a sane state to be
// persisted to the state layer.
//
// The following errors may be returned:
// - [applicationerrors.MissingStorageDirective] when one or more storage
// directives are missing that are required by the charm.
func ValidateApplicationStorageDirectives(
	charmStorageDefs map[string]internalcharm.Storage,
	directives []domainstorage.DirectiveArg,
) error {
	// seenDirectives acts as a sanity check to see if a directive by a name has
	// been witnessed.
	seenDirectives := map[string]struct{}{}
	for _, directive := range directives {
		charmStorageDef, exists := charmStorageDefs[directive.Name.String()]
		if !exists {
			return errors.Errorf(
				"invalid storage directive, charm has no storage %q",
				directive.Name,
			)
		}

		if _, seen := seenDirectives[directive.Name.String()]; seen {
			return errors.Errorf(
				"duplicate storage directive for %q exists", directive.Name,
			)
		}
		seenDirectives[directive.Name.String()] = struct{}{}

		err := validateApplicationStorageDirective(charmStorageDef, directive)
		if err != nil {
			return errors.Capture(err)
		}
	}

	// This is a sanity to check to make sure that for each required storage in
	// the charm there exists a directive for it.
	for charmStorageName, charmStorageDef := range charmStorageDefs {
		if charmStorageDef.CountMin == 0 {
			// We skip storage definitions that don't require at least one
			// storage instance. If the directive is missing that is fine.
			continue
		}

		if _, seen := seenDirectives[charmStorageName]; !seen {
			return errors.Errorf(
				"missing storage directive for charm storage %q",
				charmStorageName,
			).Add(applicationerrors.MissingStorageDirective)
		}
	}
	return nil
}

// validateApplicationStorageDirective checks a single storage directive against
// a charm storage definition. This checks the definition is inline with the
// expectations of the charm storage definition.
func validateApplicationStorageDirective(
	charmStorageDef internalcharm.Storage,
	directive domainstorage.DirectiveArg,
) error {
	minCount := uint32(0)
	if charmStorageDef.CountMin > 0 {
		minCount = uint32(charmStorageDef.CountMin)
	}
	maxCount := uint32(math.MaxUint32)
	if charmStorageDef.CountMax > 0 {
		maxCount = uint32(charmStorageDef.CountMax)
	}

	if directive.Count < minCount {
		return errors.Errorf(
			"charm requires min %d storage %q instances, %d specified",
			minCount, directive.Name, directive.Count,
		)
	}
	if directive.Count > maxCount {
		return errors.Errorf(
			"charm requires at most %d instances of storage %q, %d specified",
			maxCount, directive.Name, directive.Count,
		)
	}

	if directive.Size < charmStorageDef.MinimumSize {
		return errors.Errorf(
			"storage directive %q must be at least of size %d defined by the charm",
			directive.Name, charmStorageDef.MinimumSize,
		)
	}

	if err := directive.PoolUUID.Validate(); err != nil {
		return errors.Errorf(
			"storage directive %q pool uuid is not valid: %w",
			directive.Name, err,
		)
	}

	return nil
}
