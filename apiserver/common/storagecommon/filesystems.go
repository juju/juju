// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// FilesystemParams returns the parameters for creating or destroying the
// given filesystem.
func FilesystemParams(
	ctx context.Context,
	f state.Filesystem,
	storageInstance state.StorageInstance,
	modelUUID, controllerUUID string,
	environConfig *config.Config,
	storagePoolGetter StoragePoolGetter,
	registry storage.ProviderRegistry,
) (params.FilesystemParams, error) {

	var pool string
	var size uint64
	if stateFilesystemParams, ok := f.Params(); ok {
		pool = stateFilesystemParams.Pool
		size = stateFilesystemParams.Size
	} else {
		filesystemInfo, err := f.Info()
		if err != nil {
			return params.FilesystemParams{}, errors.Trace(err)
		}
		pool = filesystemInfo.Pool
		size = filesystemInfo.Size
	}

	filesystemTags, err := StorageTags(storageInstance, modelUUID, controllerUUID, environConfig)
	if err != nil {
		return params.FilesystemParams{}, errors.Annotate(err, "computing storage tags")
	}

	providerType, cfg, err := StoragePoolConfig(ctx, pool, storagePoolGetter, registry)
	if err != nil {
		return params.FilesystemParams{}, errors.Trace(err)
	}
	result := params.FilesystemParams{
		f.Tag().String(),
		"", // volume tag
		size,
		string(providerType),
		cfg.Attrs(),
		filesystemTags,
		nil, // attachment params set by the caller
	}

	volumeTag, err := f.Volume()
	if err == nil {
		result.VolumeTag = volumeTag.String()
	} else if err != state.ErrNoBackingVolume {
		return params.FilesystemParams{}, errors.Trace(err)
	}

	return result, nil
}

// FilesystemToState converts a params.Filesystem to state.FilesystemInfo
// and names.FilesystemTag.
func FilesystemToState(v params.Filesystem) (names.FilesystemTag, state.FilesystemInfo, error) {
	filesystemTag, err := names.ParseFilesystemTag(v.FilesystemTag)
	if err != nil {
		return names.FilesystemTag{}, state.FilesystemInfo{}, errors.Trace(err)
	}
	return filesystemTag, state.FilesystemInfo{
		v.Info.Size,
		"", // pool is set by state
		v.Info.ProviderId,
	}, nil
}

// FilesystemFromState converts a state.Filesystem to params.Filesystem.
func FilesystemFromState(f state.Filesystem) (params.Filesystem, error) {
	info, err := f.Info()
	if err != nil {
		return params.Filesystem{}, errors.Trace(err)
	}
	result := params.Filesystem{
		f.FilesystemTag().String(),
		"",
		FilesystemInfoFromState(info),
	}
	volumeTag, err := f.Volume()
	if err == nil {
		result.VolumeTag = volumeTag.String()
	} else if err != state.ErrNoBackingVolume {
		return params.Filesystem{}, errors.Trace(err)
	}
	return result, nil
}

// FilesystemInfoFromState converts a state.FilesystemInfo to params.FilesystemInfo.
func FilesystemInfoFromState(info state.FilesystemInfo) params.FilesystemInfo {
	return params.FilesystemInfo{
		info.FilesystemId,
		info.Pool,
		info.Size,
	}
}

// FilesystemAttachmentToState converts a storage.FilesystemAttachment
// to a state.FilesystemAttachmentInfo.
func FilesystemAttachmentToState(in params.FilesystemAttachment) (names.MachineTag, names.FilesystemTag, state.FilesystemAttachmentInfo, error) {
	machineTag, err := names.ParseMachineTag(in.MachineTag)
	if err != nil {
		return names.MachineTag{}, names.FilesystemTag{}, state.FilesystemAttachmentInfo{}, err
	}
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return names.MachineTag{}, names.FilesystemTag{}, state.FilesystemAttachmentInfo{}, err
	}
	info := state.FilesystemAttachmentInfo{
		in.Info.MountPoint,
		in.Info.ReadOnly,
	}
	return machineTag, filesystemTag, info, nil
}

// FilesystemAttachmentFromState converts a state.FilesystemAttachment to params.FilesystemAttachment.
func FilesystemAttachmentFromState(v state.FilesystemAttachment) (params.FilesystemAttachment, error) {
	info, err := v.Info()
	if err != nil {
		return params.FilesystemAttachment{}, errors.Trace(err)
	}
	return params.FilesystemAttachment{
		v.Filesystem().String(),
		v.Host().String(),
		FilesystemAttachmentInfoFromState(info),
	}, nil
}

// FilesystemAttachmentInfoFromState converts a state.FilesystemAttachmentInfo
// to params.FilesystemAttachmentInfo.
func FilesystemAttachmentInfoFromState(info state.FilesystemAttachmentInfo) params.FilesystemAttachmentInfo {
	return params.FilesystemAttachmentInfo{
		info.MountPoint,
		info.ReadOnly,
	}
}

// ParseFilesystemAttachmentIds parses the strings, returning machine storage IDs.
func ParseFilesystemAttachmentIds(stringIds []string) ([]params.MachineStorageId, error) {
	ids := make([]params.MachineStorageId, len(stringIds))
	for i, s := range stringIds {
		m, f, err := state.ParseFilesystemAttachmentId(s)
		if err != nil {
			return nil, err
		}
		ids[i] = params.MachineStorageId{
			MachineTag:    m.String(),
			AttachmentTag: f.String(),
		}
	}
	return ids, nil
}
