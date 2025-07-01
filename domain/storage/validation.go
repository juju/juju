// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// StoragePoolGetter provides access to a storage pool getter for validation purposes.
type StoragePoolGetter interface {
	// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolUUID(ctx context.Context, name string) (StoragePoolUUID, error)
	// GetStoragePool returns the storage pool for the specified UUID.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified UUID does not exist.
	GetStoragePool(ctx context.Context, poolUUID StoragePoolUUID) (StoragePool, error)
}

// Charm provides access to charm metadata.
type Charm interface {
	Meta() *charm.Meta
}

// StorageDirectivesValidator instances can be used to check storage
// directives are compatible with a given charm.
type StorageDirectivesValidator interface {
	ValidateStorageDirectivesAgainstCharm(ctx context.Context, allDirectives map[string]storage.Directive, charm Charm) error
}

// NewStorageDirectivesValidator creates a validator that can be used to check storage
// directives are compatible with a given charm.
func NewStorageDirectivesValidator(modelType coremodel.ModelType, registry storage.ProviderRegistry, storagePoolGetter StoragePoolGetter) (*storageDirectivesValidator, error) {
	// This should never happen, but we'll be defensive.
	if registry == nil {
		return nil, errors.New("cannot create storage directives validator with nil registry")
	}
	return &storageDirectivesValidator{
		registry:          registry,
		storagePoolGetter: storagePoolGetter,
		modelType:         modelType,
	}, nil
}

type storageDirectivesValidator struct {
	registry          storage.ProviderRegistry
	storagePoolGetter StoragePoolGetter
	modelType         coremodel.ModelType
}

// ValidateStorageDirectivesAgainstCharm validations storage directives
// against a given charm and its storage metadata.
func (v storageDirectivesValidator) ValidateStorageDirectivesAgainstCharm(
	ctx context.Context,
	allDirectives map[string]storage.Directive,
	meta *charm.Meta,
) error {
	// CAAS charms don't support volume/block storage yet.
	if v.modelType == coremodel.CAAS {
		for name, charmStorage := range meta.Storage {
			if storageKind(charmStorage.Type) != storage.StorageKindBlock {
				continue
			}
			var count uint64
			if arg, ok := allDirectives[name]; ok {
				count = arg.Count
			}
			if charmStorage.CountMin > 0 || count > 0 {
				return errors.Errorf("block storage on a container model %w", coreerrors.NotSupported)
			}
		}
	}

	for name, directive := range allDirectives {
		charmStorage, ok := meta.Storage[name]
		if !ok {
			return errors.Errorf("charm %q has no store called %q", meta.Name, name)
		}
		if charmStorage.Shared {
			// TODO(axw) implement shared storage support.
			return errors.Errorf(
				"charm %q store %q: shared storage support not implemented",
				meta.Name, name)

		}
		if err := v.validateCharmStorageCount(charmStorage, directive.Count); err != nil {
			return errors.Errorf("charm %q store %q: %w", meta.Name, name, err)
		}
		if charmStorage.MinimumSize > 0 && directive.Size < charmStorage.MinimumSize {
			return errors.Errorf(
				"charm %q store %q: minimum storage size is %s, %s specified",
				meta.Name, name,
				humanize.Bytes(charmStorage.MinimumSize*humanize.MByte),
				humanize.Bytes(directive.Size*humanize.MByte))

		}
		kind := storageKind(charmStorage.Type)
		if err := v.validateStoragePool(ctx, directive.Pool, kind); err != nil {
			return err
		}
	}
	return nil
}

func (v storageDirectivesValidator) validateCharmStorageCount(charmStorage charm.Storage, count uint64) error {
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("storage is singular, %d specified", count)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%d instances required, %d specified",
			charmStorage.CountMin, count)

	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"at most %d instances supported, %d specified",
			charmStorage.CountMax, count)

	}
	return nil
}

// validateStoragePool validates the storage pool for the model.
func (v storageDirectivesValidator) validateStoragePool(
	ctx context.Context,
	poolName string, kind storage.StorageKind,
) error {
	if poolName == "" {
		return errors.New("pool name is required")
	}
	providerType, aProvider, poolConfig, err := v.poolStorageProvider(ctx, poolName)
	if err != nil {
		return errors.Capture(err)
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
			return errors.Errorf("invalid storage config: %w", err)
		}
	}
	return nil
}

func (v storageDirectivesValidator) poolStorageProvider(
	ctx context.Context,
	poolName string,
) (storage.ProviderType, storage.Provider, storage.Attrs, error) {
	poolUUID, err := v.storagePoolGetter.GetStoragePoolUUID(ctx, poolName)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		// If there's no pool called poolName, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolName)
		aProvider, err1 := v.registry.StorageProvider(providerType)
		if err1 != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return "", nil, nil, errors.Capture(err)
		}
		return providerType, aProvider, nil, nil
	} else if err != nil {
		return "", nil, nil, errors.Capture(err)
	}
	pool, err := v.storagePoolGetter.GetStoragePool(ctx, poolUUID)
	if err != nil {
		return "", nil, nil, errors.Capture(err)
	}
	providerType := storage.ProviderType(pool.Provider)
	aProvider, err := v.registry.StorageProvider(providerType)
	if err != nil {
		return "", nil, nil, errors.Capture(err)
	}
	attrs := transform.Map(pool.Attrs, func(k, v string) (string, any) { return k, v })
	return providerType, aProvider, attrs, nil
}
