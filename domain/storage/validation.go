// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/dustin/go-humanize"
	"github.com/juju/charm/v13"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
)

// StoragePoolGetter provides access to a storage pool getter for validation purposes.
type StoragePoolGetter interface {
	GetStoragePoolByName(ctx context.Context, name string) (StoragePoolDetails, error)
}

// Charm provides access to charm metadata.
type Charm interface {
	Meta() *charm.Meta
}

// StorageConstraintsValidator instances can be used to check storage
// constraints are compatible with a given charm.
type StorageConstraintsValidator interface {
	ValidateStorageConstraintsAgainstCharm(ctx context.Context, allCons map[string]storage.Constraints, charm Charm) error
}

// NewStorageConstraintsValidator creates a validator that can be used to check storage
// constraints are compatible with a given charm.
func NewStorageConstraintsValidator(modelType coremodel.ModelType, registry storage.ProviderRegistry, storagePoolGetter StoragePoolGetter) *storageConstraintsValidator {
	return &storageConstraintsValidator{
		registry:          registry,
		storagePoolGetter: storagePoolGetter,
		modelType:         modelType,
	}
}

type storageConstraintsValidator struct {
	registry          storage.ProviderRegistry
	storagePoolGetter StoragePoolGetter
	modelType         coremodel.ModelType
}

// ValidateStorageConstraintsAgainstCharm validations storage constraints
// against a given charm and its storage metadata.
func (v storageConstraintsValidator) ValidateStorageConstraintsAgainstCharm(
	ctx context.Context,
	allCons map[string]storage.Constraints,
	charm Charm,
) error {
	charmMeta := charm.Meta()
	// CAAS charms don't support volume/block storage yet.
	if v.modelType == coremodel.CAAS {
		for name, charmStorage := range charmMeta.Storage {
			if storageKind(charmStorage.Type) != storage.StorageKindBlock {
				continue
			}
			var count uint64
			if arg, ok := allCons[name]; ok {
				count = arg.Count
			}
			if charmStorage.CountMin > 0 || count > 0 {
				return errors.NotSupportedf("block storage on a container model")
			}
		}
	}

	for name, cons := range allCons {
		charmStorage, ok := charmMeta.Storage[name]
		if !ok {
			return errors.Errorf("charm %q has no store called %q", charmMeta.Name, name)
		}
		if charmStorage.Shared {
			// TODO(axw) implement shared storage support.
			return errors.Errorf(
				"charm %q store %q: shared storage support not implemented",
				charmMeta.Name, name,
			)
		}
		if err := v.validateCharmStorageCount(charmStorage, cons.Count); err != nil {
			return errors.Annotatef(err, "charm %q store %q", charmMeta.Name, name)
		}
		if charmStorage.MinimumSize > 0 && cons.Size < charmStorage.MinimumSize {
			return errors.Errorf(
				"charm %q store %q: minimum storage size is %s, %s specified",
				charmMeta.Name, name,
				humanize.Bytes(charmStorage.MinimumSize*humanize.MByte),
				humanize.Bytes(cons.Size*humanize.MByte),
			)
		}
		kind := storageKind(charmStorage.Type)
		if err := v.validateStoragePool(ctx, cons.Pool, kind); err != nil {
			return err
		}
	}
	return nil
}

func (v storageConstraintsValidator) validateCharmStorageCount(charmStorage charm.Storage, count uint64) error {
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("storage is singular, %d specified", count)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%d instances required, %d specified",
			charmStorage.CountMin, count,
		)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"at most %d instances supported, %d specified",
			charmStorage.CountMax, count,
		)
	}
	return nil
}

// validateStoragePool validates the storage pool for the model.
func (v storageConstraintsValidator) validateStoragePool(
	ctx context.Context,
	poolName string, kind storage.StorageKind,
) error {
	if poolName == "" {
		return errors.New("pool name is required")
	}
	providerType, aProvider, poolConfig, err := v.poolStorageProvider(ctx, poolName)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure the storage provider supports the specified kind.
	kindSupported := aProvider.Supports(kind)
	if !kindSupported && kind == storage.StorageKindFilesystem {
		// Filesystems can be created if either filesystem
		// or block storage are supported. The scope of the
		// filesystem is the same as the backing volume.
		kindSupported = aProvider.Supports(storage.StorageKindBlock)
	}
	if !kindSupported {
		return errors.Errorf("%q provider does not support %q storage", providerType, kind)
	}

	if v.modelType == coremodel.CAAS {
		if err := aProvider.ValidateForK8s(poolConfig); err != nil {
			return errors.Annotatef(err, "invalid storage config")
		}
	}
	return nil
}

func (v storageConstraintsValidator) poolStorageProvider(
	ctx context.Context,
	poolName string,
) (storage.ProviderType, storage.Provider, storage.Attrs, error) {
	pool, err := v.storagePoolGetter.GetStoragePoolByName(ctx, poolName)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		// If there's no pool called poolName, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolName)
		aProvider, err1 := v.registry.StorageProvider(providerType)
		if err1 != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return "", nil, nil, errors.Trace(err)
		}
		return providerType, aProvider, nil, nil
	} else if err != nil {
		return "", nil, nil, errors.Trace(err)
	}
	providerType := storage.ProviderType(pool.Provider)
	aProvider, err := v.registry.StorageProvider(providerType)
	if err != nil {
		return "", nil, nil, errors.Trace(err)
	}
	attrs := transform.Map(pool.Attrs, func(k, v string) (string, any) { return k, v })
	return providerType, aProvider, attrs, nil
}
