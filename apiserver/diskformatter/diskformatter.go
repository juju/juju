// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

func init() {
	common.RegisterStandardFacade("DiskFormatter", 1, NewDiskFormatterAPI)
}

var logger = loggo.GetLogger("juju.apiserver.diskformatter")

// DiskFormatterAPI provides access to the DiskFormatter API facade.
type DiskFormatterAPI struct {
	st          stateInterface
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewDiskFormatterAPI creates a new server-side DiskFormatter API facade.
func NewDiskFormatterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*DiskFormatterAPI, error) {

	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	getAuthFunc := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	return &DiskFormatterAPI{
		st:          getState(st),
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

// WatchAttachedVolumes returns a NotifyWatcher for observing changes
// to the volume attachments of one or more machines.
func (a *DiskFormatterAPI) WatchAttachedVolumes(args params.Entities) (params.NotifyWatchResults, error) {
	// We say we're watching volume attachments, but in reality
	// the stimulus is block devices. Most things don't really
	// care about block devices, though, they care about "volumes
	// which are currently attached and visible to the machine".
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil || !canAccess(machineTag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		watcherId, err := a.watchOneBlockDevices(machineTag)
		if err == nil {
			result.Results[i].NotifyWatcherId = watcherId
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) watchOneBlockDevices(tag names.MachineTag) (string, error) {
	w := a.st.WatchBlockDevices(tag)
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response.
	if _, ok := <-w.Changes(); ok {
		return a.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// AttachedVolumes returns details about the volumes attached to the specified
// machines.
func (a *DiskFormatterAPI) AttachedVolumes(args params.Entities) (params.VolumeAttachmentsResults, error) {
	result := params.VolumeAttachmentsResults{
		Results: make([]params.VolumeAttachmentsResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.VolumeAttachmentsResults{}, err
	}
	one := func(entity params.Entity) ([]params.VolumeAttachment, error) {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil || !canAccess(machineTag) {
			return nil, common.ErrPerm
		}
		return a.oneAttachedVolumes(machineTag)
	}
	for i, entity := range args.Entities {
		attachments, err := one(entity)
		result.Results[i].Attachments = attachments
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) oneAttachedVolumes(tag names.MachineTag) ([]params.VolumeAttachment, error) {
	attachments, err := a.st.MachineVolumeAttachments(tag)
	if err != nil {
		return nil, err
	}
	if len(attachments) == 0 {
		return nil, nil
	}
	blockDevices, err := a.st.BlockDevices(tag)
	if err != nil {
		return nil, err
	}

	// Filter attachments without corresponding block device.
	// The worker will be notified again once the block device
	// appears.
	result := make([]params.VolumeAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		volume, err := a.st.Volume(attachment.Volume())
		if err != nil {
			return nil, errors.Trace(err)
		}
		volumeInfo, err := volume.Info()
		if err != nil {
			// Ignore unprovisioned volumes.
			continue
		}
		attachmentInfo, err := attachment.Info()
		if err != nil {
			// Ignore unprovisioned attachments.
			continue
		}
		if _, ok := matchingBlockDevice(blockDevices, volumeInfo, attachmentInfo); ok {
			result = append(result, params.VolumeAttachment{
				attachment.Volume().String(),
				volumeInfo.VolumeId,
				attachment.Machine().String(),
				"", // instance ID is not important
				attachmentInfo.DeviceName,
			})
		}
	}
	return result, nil
}

// matchingBlockDevice finds the block device that matches the
// provided volume info and volume attachment info.
func matchingBlockDevice(
	blockDevices []state.BlockDeviceInfo,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
) (*state.BlockDeviceInfo, bool) {
	for _, dev := range blockDevices {
		if volumeInfo.Serial != "" {
			if volumeInfo.Serial == dev.Serial {
				return &dev, true
			}
		} else if attachmentInfo.DeviceName == dev.DeviceName {
			return &dev, true
		}
	}
	return nil, false
}

// VolumePreparationInfo returns the information required to format the
// specified volumes.
func (a *DiskFormatterAPI) VolumePreparationInfo(args params.VolumeAttachmentIds) (params.VolumePreparationInfoResults, error) {
	result := params.VolumePreparationInfoResults{
		Results: make([]params.VolumePreparationInfoResult, len(args.Ids)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.VolumePreparationInfoResults{}, err
	}
	machineBlockDevices := make(map[names.MachineTag][]state.BlockDeviceInfo)
	one := func(id params.VolumeAttachmentId) (params.VolumePreparationInfo, error) {
		var result params.VolumePreparationInfo
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil || !canAccess(machineTag) {
			return result, common.ErrPerm
		}
		volumeTag, err := names.ParseDiskTag(id.VolumeTag)
		if err != nil {
			return result, common.ErrPerm
		}
		volume, err := a.st.Volume(volumeTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		storageTag, ok := volume.StorageInstance()
		if !ok {
			// volume is not assigned to any storage.
			return result, nil
		}
		storageInstance, err := a.st.StorageInstance(storageTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if storageInstance.Kind() != state.StorageKindFilesystem {
			// volume is assigned to a non-filesystem
			// kind storage instance.
			return result, nil
		}
		volumeInfo, err := volume.Info()
		if err != nil {
			return result, errors.Trace(err)
		}
		attachment, err := a.st.VolumeAttachment(machineTag, volumeTag)
		if err != nil {
			return result, common.ErrPerm
		}
		attachmentInfo, err := attachment.Info()
		if err != nil {
			return result, errors.Trace(err)
		}
		blockDevices, ok := machineBlockDevices[machineTag]
		if !ok {
			blockDevices, err = a.st.BlockDevices(machineTag)
			if err != nil {
				return result, errors.Trace(err)
			}
			machineBlockDevices[machineTag] = blockDevices
		}
		blockDevice, ok := matchingBlockDevice(blockDevices, volumeInfo, attachmentInfo)
		if !ok {
			// volume is not visible yet.
			return result, errors.NotFoundf(
				"volume %q on machine %q", volumeTag.Id(), machineTag.Id(),
			)
		}
		devicePath, err := storage.BlockDevicePath(storage.BlockDevice{
			blockDevice.DeviceName,
			blockDevice.Label,
			blockDevice.UUID,
			blockDevice.Serial,
			blockDevice.Size,
			blockDevice.FilesystemType,
			blockDevice.InUse,
		})
		if err != nil {
			return result, errors.Annotate(err, "determining block device path")
		}
		if blockDevice.FilesystemType == "" {
			// We've asserted previously that the volume is assigned to
			// a filesystem-kind storage instance; since the volume has
			// been observed to not have a filesystem already, we should
			// inform the clien that one should be created.
			result.NeedsFilesystem = true
			result.DevicePath = devicePath
		}
		return result, nil
	}
	for i, id := range args.Ids {
		info, err := one(id)
		result.Results[i].Result = info
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
