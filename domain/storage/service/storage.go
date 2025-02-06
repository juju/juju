// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageState defines an interface for interacting with the underlying state.
type StorageState interface {
	// GetModelDetails returns the model and controller UUID for the current model.
	GetModelDetails() (storage.ModelDetails, error)
	// ImportFilesystem associates a filesystem (either native or volume backed) hosted by a cloud provider
	// with a new storage instance (and storage pool) in a model.
	ImportFilesystem(ctx context.Context, name internalstorage.Name,
		filesystem storage.FilesystemInfo) (internalstorage.ID, error)
}

// StorageService defines a service for storage related behaviour.
type StorageService struct {
	st             State
	logger         logger.Logger
	registryGetter corestorage.ModelStorageRegistryGetter
}

// ImportFilesystem associates a filesystem (either native or volume backed) hosted by a cloud provider
// with a new storage instance (and storage pool) in a model.
// The following error types can be expected:
// - [coreerrors.NotSupported]: when the importing the kind of storage is not supported by the provider.
// - [storageerrors.InvalidPoolNameError]: when the supplied pool name is invalid.
func (s *StorageService) ImportFilesystem(
	ctx context.Context, credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter, arg ImportStorageParams,
) (internalstorage.ID, error) {
	if arg.Kind != internalstorage.StorageKindFilesystem {
		// TODO(axw) implement support for volumes.
		return "", errors.Errorf("storage kind %q not supported", arg.Kind.String()).Add(coreerrors.NotSupported)
	}
	if !internalstorage.IsValidPoolName(arg.Pool) {
		return "", errors.Errorf("pool name %q not valid", arg.Pool).Add(storageerrors.InvalidPoolNameError)
	}

	poolDetails, err := s.st.GetStoragePoolByName(ctx, arg.Pool)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		poolDetails = storage.StoragePoolDetails{
			Name:     arg.Pool,
			Provider: arg.Pool,
			Attrs:    map[string]string{},
		}
	} else if err != nil {
		return "", errors.Capture(err)
	}
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	provider, err := registry.StorageProvider(internalstorage.ProviderType(poolDetails.Provider))
	if err != nil {
		return "", errors.Capture(err)
	}

	details, err := s.st.GetModelDetails()
	if err != nil {
		return "", errors.Capture(err)
	}
	resourceTags := map[string]string{
		tags.JujuModel:      details.ModelUUID,
		tags.JujuController: details.ControllerUUID,
	}
	filesystemInfo := storage.FilesystemInfo{Pool: arg.Pool}

	// If the storage provider supports filesystems, import the filesystem,
	// otherwise import a volume which will back a filesystem.
	invalidatorFunc, err := credentialInvalidatorGetter()
	if err != nil {
		return "", errors.Capture(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)

	var attr map[string]any
	if len(poolDetails.Attrs) > 0 {
		attr = transform.Map(poolDetails.Attrs, func(k, v string) (string, any) { return k, v })
	}
	cfg, err := internalstorage.NewConfig(poolDetails.Name, internalstorage.ProviderType(poolDetails.Provider), attr)
	if err != nil {
		return "", errors.Capture(err)
	}
	if provider.Supports(internalstorage.StorageKindFilesystem) {
		filesystemSource, err := provider.FilesystemSource(cfg)
		if err != nil {
			return "", errors.Capture(err)
		}
		filesystemImporter, ok := filesystemSource.(internalstorage.FilesystemImporter)
		if !ok {
			return "", errors.Errorf(
				"importing filesystem with storage provider %q not supported",
				cfg.Provider(),
			).Add(coreerrors.NotSupported)
		}
		info, err := filesystemImporter.ImportFilesystem(callCtx, arg.ProviderId, resourceTags)
		if err != nil {
			return "", errors.Errorf("importing filesystem: %w", err)
		}
		filesystemInfo.FilesystemId = arg.ProviderId
		filesystemInfo.Size = info.Size
	} else {
		volumeSource, err := provider.VolumeSource(cfg)
		if err != nil {
			return "", errors.Capture(err)
		}
		volumeImporter, ok := volumeSource.(internalstorage.VolumeImporter)
		if !ok {
			return "", errors.Errorf(
				"importing volume with storage provider %q not supported",
				cfg.Provider(),
			).Add(coreerrors.NotSupported)
		}
		info, err := volumeImporter.ImportVolume(callCtx, arg.ProviderId, resourceTags)
		if err != nil {
			return "", errors.Errorf("importing volume: %w", err)
		}
		filesystemInfo.BackingVolume = &internalstorage.VolumeInfo{
			HardwareId: info.HardwareId,
			WWN:        info.WWN,
			Size:       info.Size,
			VolumeId:   info.VolumeId,
			Persistent: info.Persistent,
		}
		filesystemInfo.Size = info.Size
	}

	return s.st.ImportFilesystem(ctx, arg.StorageName, filesystemInfo)
}
