// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
)

type volumeAlreadyProvisionedError struct {
	error
}

// IsVolumeAlreadyProvisioned returns true if the specified error
// is caused by a volume already being provisioned.
func IsVolumeAlreadyProvisioned(err error) bool {
	_, ok := err.(*volumeAlreadyProvisionedError)
	return ok
}

// VolumeParams returns the parameters for creating or destroying
// the given volume.
func VolumeParams(
	v state.Volume,
	storageInstance state.StorageInstance,
	environConfig *config.Config,
	poolManager poolmanager.PoolManager,
) (params.VolumeParams, error) {

	var pool string
	var size uint64
	if stateVolumeParams, ok := v.Params(); ok {
		pool = stateVolumeParams.Pool
		size = stateVolumeParams.Size
	} else {
		volumeInfo, err := v.Info()
		if err != nil {
			return params.VolumeParams{}, errors.Trace(err)
		}
		pool = volumeInfo.Pool
		size = volumeInfo.Size
	}

	volumeTags, err := storageTags(storageInstance, environConfig)
	if err != nil {
		return params.VolumeParams{}, errors.Annotate(err, "computing storage tags")
	}

	providerType, cfg, err := StoragePoolConfig(pool, poolManager)
	if err != nil {
		return params.VolumeParams{}, errors.Trace(err)
	}
	return params.VolumeParams{
		v.Tag().String(),
		size,
		string(providerType),
		cfg.Attrs(),
		volumeTags,
		nil, // attachment params set by the caller
	}, nil
}

// StoragePoolConfig returns the storage provider type and
// configuration for a named storage pool. If there is no
// such pool with the specified name, but it identifies a
// storage provider, then that type will be returned with a
// nil configuration.
func StoragePoolConfig(name string, poolManager poolmanager.PoolManager) (storage.ProviderType, *storage.Config, error) {
	pool, err := poolManager.Get(name)
	if errors.IsNotFound(err) {
		// If not a storage pool, then maybe a provider type.
		providerType := storage.ProviderType(name)
		if _, err1 := registry.StorageProvider(providerType); err1 != nil {
			return "", nil, errors.Trace(err)
		}
		return providerType, &storage.Config{}, nil
	} else if err != nil {
		return "", nil, errors.Annotatef(err, "getting pool %q", name)
	}
	return pool.Provider(), pool, nil
}

// VolumesToState converts a slice of params.Volume to a mapping
// of volume tags to state.VolumeInfo.
func VolumesToState(in []params.Volume) (map[names.VolumeTag]state.VolumeInfo, error) {
	m := make(map[names.VolumeTag]state.VolumeInfo)
	for _, v := range in {
		tag, volumeInfo, err := VolumeToState(v)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m[tag] = volumeInfo
	}
	return m, nil
}

// VolumeToState converts a params.Volume to state.VolumeInfo
// and names.VolumeTag.
func VolumeToState(v params.Volume) (names.VolumeTag, state.VolumeInfo, error) {
	if v.VolumeTag == "" {
		return names.VolumeTag{}, state.VolumeInfo{}, errors.New("Tag is empty")
	}
	volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
	if err != nil {
		return names.VolumeTag{}, state.VolumeInfo{}, errors.Trace(err)
	}
	return volumeTag, state.VolumeInfo{
		v.Info.HardwareId,
		v.Info.Size,
		"", // pool is set by state
		v.Info.VolumeId,
		v.Info.Persistent,
	}, nil
}

// VolumeFromState converts a state.Volume to params.Volume.
func VolumeFromState(v state.Volume) (params.Volume, error) {
	info, err := v.Info()
	if err != nil {
		return params.Volume{}, errors.Trace(err)
	}
	return params.Volume{
		v.VolumeTag().String(),
		params.VolumeInfo{
			info.VolumeId,
			info.HardwareId,
			info.Size,
			info.Persistent,
		},
	}, nil
}

// VolumeAttachmentFromState converts a state.VolumeAttachment to params.VolumeAttachment.
func VolumeAttachmentFromState(v state.VolumeAttachment) (params.VolumeAttachment, error) {
	info, err := v.Info()
	if err != nil {
		return params.VolumeAttachment{}, errors.Trace(err)
	}
	return params.VolumeAttachment{
		v.Volume().String(),
		v.Machine().String(),
		params.VolumeAttachmentInfo{
			info.DeviceName,
			info.BusAddress,
			info.ReadOnly,
		},
	}, nil
}

// VolumeAttachmentInfosToState converts a map of volume tags to
// params.VolumeAttachmentInfo to a map of volume tags to
// state.VolumeAttachmentInfo.
func VolumeAttachmentInfosToState(in map[string]params.VolumeAttachmentInfo) (map[names.VolumeTag]state.VolumeAttachmentInfo, error) {
	m := make(map[names.VolumeTag]state.VolumeAttachmentInfo)
	for k, v := range in {
		volumeTag, err := names.ParseVolumeTag(k)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m[volumeTag] = VolumeAttachmentInfoToState(v)
	}
	return m, nil
}

// VolumeAttachmentToState converts a params.VolumeAttachment
// to a state.VolumeAttachmentInfo and tags.
func VolumeAttachmentToState(in params.VolumeAttachment) (names.MachineTag, names.VolumeTag, state.VolumeAttachmentInfo, error) {
	machineTag, err := names.ParseMachineTag(in.MachineTag)
	if err != nil {
		return names.MachineTag{}, names.VolumeTag{}, state.VolumeAttachmentInfo{}, err
	}
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return names.MachineTag{}, names.VolumeTag{}, state.VolumeAttachmentInfo{}, err
	}
	info := VolumeAttachmentInfoToState(in.Info)
	return machineTag, volumeTag, info, nil
}

// VolumeAttachmentInfoToState converts a params.VolumeAttachmentInfo
// to a state.VolumeAttachmentInfo.
func VolumeAttachmentInfoToState(in params.VolumeAttachmentInfo) state.VolumeAttachmentInfo {
	return state.VolumeAttachmentInfo{
		in.DeviceName,
		in.BusAddress,
		in.ReadOnly,
	}
}

// ParseVolumeAttachmentIds parses the strings, returning machine storage IDs.
func ParseVolumeAttachmentIds(stringIds []string) ([]params.MachineStorageId, error) {
	ids := make([]params.MachineStorageId, len(stringIds))
	for i, s := range stringIds {
		m, v, err := state.ParseVolumeAttachmentId(s)
		if err != nil {
			return nil, err
		}
		ids[i] = params.MachineStorageId{
			MachineTag:    m.String(),
			AttachmentTag: v.String(),
		}
	}
	return ids, nil
}
