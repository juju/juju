// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// VolumeParams returns the parameters for creating or destroying
// the given volume.
func VolumeParams(
	ctx context.Context,
	v state.Volume,
	storageInstance state.StorageInstance,
	modelUUID, controllerUUID string,
	environConfig *config.Config,
	storagePoolGetter StoragePoolGetter,
	registry storage.ProviderRegistry,
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

	volumeTags, err := StorageTags(storageInstance, modelUUID, controllerUUID, environConfig)
	if err != nil {
		return params.VolumeParams{}, errors.Annotate(err, "computing storage tags")
	}

	providerType, cfg, err := StoragePoolConfig(ctx, pool, storagePoolGetter, registry)
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
func StoragePoolConfig(ctx context.Context, name string, storagePoolGetter StoragePoolGetter, registry storage.ProviderRegistry) (storage.ProviderType, *storage.Config, error) {
	pool, err := storagePoolGetter.GetStoragePoolByName(ctx, name)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		// If not a storage pool, then maybe a provider type.
		providerType := storage.ProviderType(name)
		if _, err1 := registry.StorageProvider(providerType); err1 != nil {
			return "", nil, errors.Trace(err)
		}
		return providerType, &storage.Config{}, nil
	} else if err != nil {
		return "", nil, errors.Annotatef(err, "getting pool %q", name)
	}
	var attr map[string]any
	if len(pool.Attrs) > 0 {
		attr = make(map[string]any, len(pool.Attrs))
		for k, v := range pool.Attrs {
			attr[k] = v
		}
	}
	providerType := storage.ProviderType(pool.Provider)
	cfg, err := storage.NewConfig(pool.Name, providerType, attr)
	if err != nil {
		return "", nil, internalerrors.Capture(err)
	}
	return providerType, cfg, nil
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
		v.Info.WWN,
		v.Info.Size,
		"", // pool is set by state
		v.Info.ProviderId,
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
		VolumeInfoFromState(info),
	}, nil
}

// VolumeInfoFromState converts a state.VolumeInfo to params.VolumeInfo.
func VolumeInfoFromState(info state.VolumeInfo) params.VolumeInfo {
	return params.VolumeInfo{
		info.VolumeId,
		info.HardwareId,
		info.WWN,
		info.Pool,
		info.Size,
		info.Persistent,
	}
}

// VolumeAttachmentPlanFromState converts a state.VolumeAttachmentPlan to params.VolumeAttachmentPlan.
func VolumeAttachmentPlanFromState(v state.VolumeAttachmentPlan) (params.VolumeAttachmentPlan, error) {
	planInfo, err := v.PlanInfo()
	if err != nil {
		return params.VolumeAttachmentPlan{}, errors.Trace(err)
	}

	blockInfo, err := v.BlockDeviceInfo()
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return params.VolumeAttachmentPlan{}, errors.Trace(err)
		}
	}
	return params.VolumeAttachmentPlan{
		VolumeTag:   v.Volume().String(),
		MachineTag:  v.Machine().String(),
		Life:        life.Value(v.Life().String()),
		PlanInfo:    VolumeAttachmentPlanInfoFromState(planInfo),
		BlockDevice: VolumeAttachmentPlanBlockParamsFromState(blockInfo),
	}, nil
}

func VolumeAttachmentPlanBlockParamsFromState(blockInfo state.BlockDeviceInfo) params.BlockDevice {
	return params.BlockDevice{
		DeviceName:     blockInfo.DeviceName,
		DeviceLinks:    blockInfo.DeviceLinks,
		Label:          blockInfo.Label,
		UUID:           blockInfo.UUID,
		HardwareId:     blockInfo.HardwareId,
		WWN:            blockInfo.WWN,
		BusAddress:     blockInfo.BusAddress,
		Size:           blockInfo.Size,
		FilesystemType: blockInfo.FilesystemType,
		InUse:          blockInfo.InUse,
		MountPoint:     blockInfo.MountPoint,
	}
}

func VolumeAttachmentPlanBlockInfoFromState(blockInfo state.BlockDeviceInfo) blockdevice.BlockDevice {
	return blockdevice.BlockDevice{
		DeviceName:     blockInfo.DeviceName,
		DeviceLinks:    blockInfo.DeviceLinks,
		Label:          blockInfo.Label,
		UUID:           blockInfo.UUID,
		HardwareId:     blockInfo.HardwareId,
		WWN:            blockInfo.WWN,
		BusAddress:     blockInfo.BusAddress,
		SizeMiB:        blockInfo.Size,
		FilesystemType: blockInfo.FilesystemType,
		InUse:          blockInfo.InUse,
		MountPoint:     blockInfo.MountPoint,
	}
}

func VolumeAttachmentPlanInfoFromState(planInfo state.VolumeAttachmentPlanInfo) params.VolumeAttachmentPlanInfo {
	return params.VolumeAttachmentPlanInfo{
		DeviceType:       planInfo.DeviceType,
		DeviceAttributes: planInfo.DeviceAttributes,
	}
}

// VolumeAttachmentFromState converts a state.VolumeAttachment to params.VolumeAttachment.
func VolumeAttachmentFromState(v state.VolumeAttachment) (params.VolumeAttachment, error) {
	info, err := v.Info()
	if err != nil {
		return params.VolumeAttachment{}, errors.Trace(err)
	}
	return params.VolumeAttachment{
		v.Volume().String(),
		v.Host().String(),
		VolumeAttachmentInfoFromState(info),
	}, nil
}

// VolumeAttachmentInfoFromState converts a state.VolumeAttachmentInfo to params.VolumeAttachmentInfo.
func VolumeAttachmentInfoFromState(info state.VolumeAttachmentInfo) params.VolumeAttachmentInfo {
	planInfo := &params.VolumeAttachmentPlanInfo{}
	if info.PlanInfo != nil {
		planInfo.DeviceType = info.PlanInfo.DeviceType
		planInfo.DeviceAttributes = info.PlanInfo.DeviceAttributes
	} else {
		planInfo = nil
	}
	return params.VolumeAttachmentInfo{
		info.DeviceName,
		info.DeviceLink,
		info.BusAddress,
		info.ReadOnly,
		planInfo,
	}
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

func VolumeAttachmentPlanToState(in params.VolumeAttachmentPlan) (names.MachineTag, names.VolumeTag, state.VolumeAttachmentPlanInfo, state.BlockDeviceInfo, error) {
	machineTag, err := names.ParseMachineTag(in.MachineTag)
	if err != nil {
		return names.MachineTag{}, names.VolumeTag{}, state.VolumeAttachmentPlanInfo{}, state.BlockDeviceInfo{}, err
	}
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return names.MachineTag{}, names.VolumeTag{}, state.VolumeAttachmentPlanInfo{}, state.BlockDeviceInfo{}, err
	}
	info := VolumeAttachmentPlanInfoToState(in.PlanInfo)
	blockInfo := BlockDeviceInfoToState(in.BlockDevice)
	return machineTag, volumeTag, info, blockInfo, nil
}

func BlockDeviceInfoToState(in params.BlockDevice) state.BlockDeviceInfo {
	return state.BlockDeviceInfo{
		DeviceName:     in.DeviceName,
		DeviceLinks:    in.DeviceLinks,
		Label:          in.Label,
		UUID:           in.UUID,
		HardwareId:     in.HardwareId,
		WWN:            in.WWN,
		BusAddress:     in.BusAddress,
		Size:           in.Size,
		FilesystemType: in.FilesystemType,
		InUse:          in.InUse,
		MountPoint:     in.MountPoint,
	}
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

func VolumeAttachmentPlanInfoToState(in params.VolumeAttachmentPlanInfo) state.VolumeAttachmentPlanInfo {
	deviceType := in.DeviceType
	if deviceType == "" {
		deviceType = storage.DeviceTypeLocal
	}

	return state.VolumeAttachmentPlanInfo{
		DeviceType:       deviceType,
		DeviceAttributes: in.DeviceAttributes,
	}
}

// VolumeAttachmentInfoToState converts a params.VolumeAttachmentInfo
// to a state.VolumeAttachmentInfo.
func VolumeAttachmentInfoToState(in params.VolumeAttachmentInfo) state.VolumeAttachmentInfo {
	planInfo := &state.VolumeAttachmentPlanInfo{}
	if in.PlanInfo != nil {
		planInfo.DeviceAttributes = in.PlanInfo.DeviceAttributes
		planInfo.DeviceType = in.PlanInfo.DeviceType
	} else {
		planInfo = nil
	}
	return state.VolumeAttachmentInfo{
		in.DeviceName,
		in.DeviceLink,
		in.BusAddress,
		in.ReadOnly,
		planInfo,
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
