// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
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
	return s.st.GetApplicationStorageDirectives(ctx, uuid)
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
) ([]application.CreateApplicationStorageDirectiveArg, error) {
	if len(charmMetaStorage) == 0 {
		return nil, nil
	}

	modelStoragePools, err := s.st.GetModelStoragePools(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting default storage provisioners for model: %w", err,
		)
	}

	rval := make([]application.CreateApplicationStorageDirectiveArg, 0, len(charmMetaStorage))
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
) application.CreateApplicationStorageDirectiveArg {
	rval := application.CreateApplicationStorageDirectiveArg{
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
	applicationArgs []application.CreateApplicationStorageDirectiveArg,
) []application.StorageDirective {
	rval := make([]application.StorageDirective, 0, len(applicationArgs))
	for _, arg := range applicationArgs {
		rval = append(rval, application.StorageDirective{
			CharmMetadataName: charmMetadataName,
			Name:              arg.Name,
			Count:             arg.Count,
			CharmStorageType:  charm.StorageType(charmStorage[arg.Name.String()].Type),
			PoolUUID:          arg.PoolUUID,
			Size:              arg.Size,
		})
	}

	return rval
}

// ValidateApplicationStorageDirectiveOverrides checks a set of storage
// directive overrides to make sure they are valid with respect to the charms
// storage definitions.
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
func validateApplicationStorageDirectiveOverride(
	ctx context.Context,
	charmStorageDef internalcharm.Storage,
	override StorageDirectiveOverride,
	poolProvider StoragePoolProvider,
) error {
	if override.Count != nil {
		minCount := uint32(0)
		if charmStorageDef.CountMin > 0 {
			minCount = uint32(charmStorageDef.CountMin)
		}
		maxCount := uint32(0)
		if charmStorageDef.CountMax > 0 {
			maxCount = uint32(charmStorageDef.CountMax)
		}

		if *override.Count < minCount {
			return errors.Errorf(
				"storage directive count %d is less than the charm minimum of %d",
				*override.Count, minCount,
			)
		}
		if *override.Count > maxCount {
			return errors.Errorf(
				"storage directive count %d is greater than the charm maximum of %d",
				*override.Count, maxCount,
			)
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
		supports, err := poolProvider.CheckPoolSupportsCharmStorage(
			ctx, *override.PoolUUID, charmStorageDef.Type,
		)
		if err != nil {
			return errors.Errorf(
				"checking storage directive pool %q supports charm storage %q",
				*override.PoolUUID, charmStorageDef.Type,
			)
		}

		if !supports &&
			charmStorageDef.Type == internalcharm.StorageFilesystem {
			// TODO(storage): unify these checks with
			// CalculateStorageInstaceComposition.
			supports, err = poolProvider.CheckPoolSupportsCharmStorage(
				ctx, *override.PoolUUID, internalcharm.StorageBlock,
			)
			if err != nil {
				return errors.Errorf(
					"checking storage directive pool %q supports charm storage %q",
					*override.PoolUUID, internalcharm.StorageBlock,
				)
			}
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
