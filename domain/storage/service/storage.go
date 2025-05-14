// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
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
	ImportFilesystem(ctx context.Context, name corestorage.Name,
		filesystem storage.FilesystemInfo) (corestorage.ID, error)
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
func (s *StorageService) ImportFilesystem(ctx context.Context, arg ImportStorageParams) (corestorage.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if arg.Kind != internalstorage.StorageKindFilesystem {
		// TODO(axw) implement support for volumes.
		return "", errors.Errorf("storage kind %q not supported", arg.Kind.String()).Add(coreerrors.NotSupported)
	}
	if !internalstorage.IsValidPoolName(arg.Pool) {
		return "", errors.Errorf("pool name %q not valid", arg.Pool).Add(storageerrors.InvalidPoolNameError)
	}
	if err := arg.StorageName.Validate(); err != nil {
		return "", errors.Capture(err)
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

	var attr map[string]any
	if len(poolDetails.Attrs) > 0 {
		attr = transform.Map(poolDetails.Attrs, func(k, v string) (string, any) { return k, v })
	}
	cfg, err := internalstorage.NewConfig(poolDetails.Name, internalstorage.ProviderType(poolDetails.Provider), attr)
	if err != nil {
		return "", errors.Capture(err)
	}
	filesystemInfo, err := s.importStorageFromProvider(ctx, cfg, arg.ProviderId)
	if err != nil {
		return "", errors.Capture(err)
	}

	return s.st.ImportFilesystem(ctx, arg.StorageName, *filesystemInfo)
}

func (s *StorageService) importStorageFromProvider(ctx context.Context, cfg *internalstorage.Config, providerID string) (*storage.FilesystemInfo, error) {
	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	provider, err := registry.StorageProvider(cfg.Provider())
	if err != nil {
		return nil, errors.Capture(err)
	}

	details, err := s.st.GetModelDetails()
	if err != nil {
		return nil, errors.Capture(err)
	}
	resourceTags := map[string]string{
		tags.JujuModel:      details.ModelUUID,
		tags.JujuController: details.ControllerUUID,
	}

	// If the storage provider supports filesystems, import the filesystem,
	// otherwise import a volume which will back a filesystem.
	var filesystemInfo *storage.FilesystemInfo
	if provider.Supports(internalstorage.StorageKindFilesystem) {
		filesystemInfo, err = s.importFilesystemFromProvider(ctx, provider, cfg, providerID, resourceTags)
	} else {
		filesystemInfo, err = s.importVolumeFromProvider(ctx, provider, cfg, providerID, resourceTags)
	}
	if err != nil {
		return nil, errors.Capture(err)
	}
	return filesystemInfo, nil
}

func (s *StorageService) importFilesystemFromProvider(ctx context.Context, provider internalstorage.Provider, cfg *internalstorage.Config, providerID string, resourceTags map[string]string) (*storage.FilesystemInfo, error) {
	filesystemSource, err := provider.FilesystemSource(cfg)
	if err != nil {
		return nil, errors.Capture(err)
	}
	filesystemImporter, ok := filesystemSource.(internalstorage.FilesystemImporter)
	if !ok {
		return nil, errors.Errorf(
			"importing filesystem with storage provider %q not supported",
			cfg.Provider(),
		).Add(coreerrors.NotSupported)
	}
	info, err := filesystemImporter.ImportFilesystem(ctx, providerID, resourceTags)
	if err != nil {
		return nil, errors.Errorf("importing filesystem: %w", err)
	}
	filesystemInfo := &storage.FilesystemInfo{
		Pool: cfg.Name(),
		FilesystemInfo: internalstorage.FilesystemInfo{
			FilesystemId: info.FilesystemId,
			Size:         info.Size,
		},
	}
	return filesystemInfo, nil
}

func (s *StorageService) importVolumeFromProvider(
	ctx context.Context, provider internalstorage.Provider, cfg *internalstorage.Config,
	providerID string, resourceTags map[string]string,
) (*storage.FilesystemInfo, error) {
	volumeSource, err := provider.VolumeSource(cfg)
	if err != nil {
		return nil, errors.Capture(err)
	}
	volumeImporter, ok := volumeSource.(internalstorage.VolumeImporter)
	if !ok {
		return nil, errors.Errorf(
			"importing volume with storage provider %q not supported",
			cfg.Provider(),
		).Add(coreerrors.NotSupported)
	}
	info, err := volumeImporter.ImportVolume(ctx, providerID, resourceTags)
	if err != nil {
		return nil, errors.Errorf("importing volume: %w", err)
	}
	filesystemInfo := &storage.FilesystemInfo{
		Pool: cfg.Name(),
		FilesystemInfo: internalstorage.FilesystemInfo{
			Size: info.Size,
		},
		BackingVolume: &internalstorage.VolumeInfo{
			HardwareId: info.HardwareId,
			WWN:        info.WWN,
			Size:       info.Size,
			VolumeId:   info.VolumeId,
			Persistent: info.Persistent,
		},
	}
	return filesystemInfo, nil
}
