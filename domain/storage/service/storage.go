// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
)

// StorageState defines an interface for interacting with the underlying state.
type StorageState interface {
	// GetModelDetails returns the model and controller UUID for the current model.
	GetModelDetails() (storage.ModelDetails, error)
	// ImportFilesystem associates a filesystem (either native or volume backed) hosted by a cloud provider
	// with a new storage instance (and storage pool) in a model.
	ImportFilesystem(ctx context.Context, name corestorage.Name,
		filesystem storage.FilesystemInfo) (corestorage.ID, error)

	// GetAllStorageInstances returns a list of storage instances in the model.
	GetAllStorageInstances(ctx context.Context) ([]internal.StorageInstanceDetails, error)

	// GetVolumeWithAttachments returns a map of volume storage IDs to their
	// information including attachments.
	GetVolumeWithAttachments(
		ctx context.Context, storageInstanceIDs ...string,
	) (map[string]internal.VolumeDetails, error)

	// GetFilesystemWithAttachments returns a map of filesystem storage IDs to their
	// information including attachments.
	GetFilesystemWithAttachments(
		ctx context.Context, storageInstanceIDs ...string,
	) (map[string]internal.FilesystemDetails, error)
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
	// TODO: Implement this function.
	return "", errors.New("ImportFilesystem is not implemented yet").Add(coreerrors.NotImplemented)

	// ctx, span := trace.Start(ctx, trace.NameFromFunc())
	// defer span.End()

	// if arg.Kind != internalstorage.StorageKindFilesystem {
	// 	// TODO(axw) implement support for volumes.
	// 	return "", errors.Errorf("storage kind %q not supported", arg.Kind.String()).Add(coreerrors.NotSupported)
	// }
	// if !internalstorage.IsValidPoolName(arg.Pool) {
	// 	return "", errors.Errorf("pool name %q not valid", arg.Pool).Add(storageerrors.InvalidPoolNameError)
	// }
	// if err := arg.StorageName.Validate(); err != nil {
	// 	return "", errors.Capture(err)
	// }

	// poolDetails, err := s.st.GetStoragePoolByName(ctx, arg.Pool)
	// if errors.Is(err, storageerrors.PoolNotFoundError) {
	// 	poolDetails = storage.StoragePool{
	// 		Name:     arg.Pool,
	// 		Provider: arg.Pool,
	// 		Attrs:    map[string]string{},
	// 	}
	// } else if err != nil {
	// 	return "", errors.Capture(err)
	// }

	// var attr map[string]any
	// if len(poolDetails.Attrs) > 0 {
	// 	attr = transform.Map(poolDetails.Attrs, func(k, v string) (string, any) { return k, v })
	// }
	// cfg, err := internalstorage.NewConfig(poolDetails.Name, internalstorage.ProviderType(poolDetails.Provider), attr)
	// if err != nil {
	// 	return "", errors.Capture(err)
	// }
	// filesystemInfo, err := s.importStorageFromProvider(ctx, cfg, arg.ProviderId)
	// if err != nil {
	// 	return "", errors.Capture(err)
	// }

	// return s.st.ImportFilesystem(ctx, arg.StorageName, *filesystemInfo)
}

// func (s *StorageService) importStorageFromProvider(ctx context.Context, cfg *internalstorage.Config, providerID string) (*storage.FilesystemInfo, error) {
// 	registry, err := s.registryGetter.GetStorageRegistry(ctx)
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}
// 	provider, err := registry.StorageProvider(cfg.Provider())
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}

// 	details, err := s.st.GetModelDetails()
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}
// 	resourceTags := map[string]string{
// 		tags.JujuModel:      details.ModelUUID,
// 		tags.JujuController: details.ControllerUUID,
// 	}

// 	// If the storage provider supports filesystems, import the filesystem,
// 	// otherwise import a volume which will back a filesystem.
// 	var filesystemInfo *storage.FilesystemInfo
// 	if provider.Supports(internalstorage.StorageKindFilesystem) {
// 		filesystemInfo, err = s.importFilesystemFromProvider(ctx, provider, cfg, providerID, resourceTags)
// 	} else {
// 		filesystemInfo, err = s.importVolumeFromProvider(ctx, provider, cfg, providerID, resourceTags)
// 	}
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}
// 	return filesystemInfo, nil
// }

// func (s *StorageService) importFilesystemFromProvider(ctx context.Context, provider internalstorage.Provider, cfg *internalstorage.Config, providerID string, resourceTags map[string]string) (*storage.FilesystemInfo, error) {
// 	filesystemSource, err := provider.FilesystemSource(cfg)
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}
// 	filesystemImporter, ok := filesystemSource.(internalstorage.FilesystemImporter)
// 	if !ok {
// 		return nil, errors.Errorf(
// 			"importing filesystem with storage provider %q not supported",
// 			cfg.Provider(),
// 		).Add(coreerrors.NotSupported)
// 	}
// 	info, err := filesystemImporter.ImportFilesystem(ctx, providerID, resourceTags)
// 	if err != nil {
// 		return nil, errors.Errorf("importing filesystem: %w", err)
// 	}
// 	filesystemInfo := &storage.FilesystemInfo{
// 		Pool: cfg.Name(),
// 		FilesystemInfo: internalstorage.FilesystemInfo{
// 			FilesystemId: info.FilesystemId,
// 			Size:         info.Size,
// 		},
// 	}
// 	return filesystemInfo, nil
// }

// func (s *StorageService) importVolumeFromProvider(
// 	ctx context.Context, provider internalstorage.Provider, cfg *internalstorage.Config,
// 	providerID string, resourceTags map[string]string,
// ) (*storage.FilesystemInfo, error) {
// 	volumeSource, err := provider.VolumeSource(cfg)
// 	if err != nil {
// 		return nil, errors.Capture(err)
// 	}
// 	volumeImporter, ok := volumeSource.(internalstorage.VolumeImporter)
// 	if !ok {
// 		return nil, errors.Errorf(
// 			"importing volume with storage provider %q not supported",
// 			cfg.Provider(),
// 		).Add(coreerrors.NotSupported)
// 	}
// 	info, err := volumeImporter.ImportVolume(ctx, providerID, resourceTags)
// 	if err != nil {
// 		return nil, errors.Errorf("importing volume: %w", err)
// 	}
// 	filesystemInfo := &storage.FilesystemInfo{
// 		Pool: cfg.Name(),
// 		FilesystemInfo: internalstorage.FilesystemInfo{
// 			Size: info.Size,
// 		},
// 		BackingVolume: &internalstorage.VolumeInfo{
// 			HardwareId: info.HardwareId,
// 			WWN:        info.WWN,
// 			Size:       info.Size,
// 			VolumeId:   info.VolumeId,
// 			Persistent: info.Persistent,
// 		},
// 	}
// 	return filesystemInfo, nil
// }

// GetAllStorageInstances returns a list of storage instances in the model.
func (s *StorageService) GetAllStorageInstances(ctx context.Context) ([]storage.StorageInstanceDetails, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	sInstances, err := s.st.GetAllStorageInstances(ctx)
	if err != nil {
		return nil, errors.Errorf("listing storage instances: %w", err)
	}
	result := make([]storage.StorageInstanceDetails, 0, len(sInstances))
	for _, si := range sInstances {
		result = append(result, storage.StorageInstanceDetails{
			UUID:       storage.StorageInstanceUUID(si.UUID),
			ID:         si.ID,
			Owner:      si.Owner,
			Kind:       si.Kind,
			Life:       si.Life,
			Persistent: si.Persistent,
		})
	}
	return result, nil
}

// GetVolumeWithAttachments returns a map of volume storage IDs to their
// information including attachments.
func (s *StorageService) GetVolumeWithAttachments(
	ctx context.Context, uuids ...storage.StorageInstanceUUID,
) (map[string]storage.VolumeDetails, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	var siUUIDs []string
	for _, uuid := range uuids {
		if err := uuid.Validate(); err != nil {
			return nil, errors.Errorf("invalid storage instance UUID %q: %w", uuid, err)
		}
		siUUIDs = append(siUUIDs, uuid.String())
	}

	vols, err := s.st.GetVolumeWithAttachments(ctx, siUUIDs...)
	if err != nil {
		return nil, errors.Errorf("listing volume attachments: %w", err)
	}

	result := make(map[string]storage.VolumeDetails, len(vols))
	for id, v := range vols {
		vd := storage.VolumeDetails{
			StorageID: v.StorageID,
		}
		vd.Status, err = status.DecodeVolumeStatus(v.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		for _, att := range v.Attachments {
			va := storage.VolumeAttachmentDetails{
				AttachmentDetails: storage.AttachmentDetails{
					Life:    att.Life,
					Unit:    att.Unit,
					Machine: att.Machine,
				},
				BlockDeviceUUID: att.BlockDeviceUUID,
			}
			vd.Attachments = append(vd.Attachments, va)
		}
		result[id] = vd
	}
	return result, nil
}

// GetFilesystemWithAttachments returns a map of filesystem storage IDs to their
// information including attachments.
func (s *StorageService) GetFilesystemWithAttachments(
	ctx context.Context, uuids ...storage.StorageInstanceUUID,
) (map[string]storage.FilesystemDetails, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	var siUUIDs []string
	for _, uuid := range uuids {
		if err := uuid.Validate(); err != nil {
			return nil, errors.Errorf("invalid storage instance UUID %q: %w", uuid, err)
		}
		siUUIDs = append(siUUIDs, uuid.String())
	}

	fss, err := s.st.GetFilesystemWithAttachments(ctx, siUUIDs...)
	if err != nil {
		return nil, errors.Errorf("listing filesystem attachments: %w", err)
	}

	result := make(map[string]storage.FilesystemDetails, len(fss))
	for id, fs := range fss {
		fd := storage.FilesystemDetails{
			StorageID: fs.StorageID,
		}
		fd.Status, err = status.DecodeFilesystemStatus(fs.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		for _, att := range fs.Attachments {
			fa := storage.FilesystemAttachmentDetails{
				AttachmentDetails: storage.AttachmentDetails{
					Life:    att.Life,
					Unit:    att.Unit,
					Machine: att.Machine,
				},
				MountPoint: att.MountPoint,
			}
			fd.Attachments = append(fd.Attachments, fa)
		}
		result[id] = fd
	}
	return result, nil
}
