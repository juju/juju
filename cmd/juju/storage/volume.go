// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/common"
)

const volumeCmdDoc = `
"juju storage volume" is used to manage storage volumes in
 the Juju environment.
`

const volumeCmdPurpose = "manage storage volumes"

// NewVolumeSuperCommand creates the storage volume super subcommand and
// registers the subcommands that it supports.
func NewVolumeSuperCommand() cmd.Command {
	poolcmd := jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
		Name:        "volume",
		Doc:         volumeCmdDoc,
		UsagePrefix: "juju storage",
		Purpose:     volumeCmdPurpose,
	})
	poolcmd.Register(envcmd.Wrap(&VolumeListCommand{}))
	return poolcmd
}

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}

// VolumeInfo defines the serialization behaviour for storage volume.
type VolumeInfo struct {
	// from params.Volume. This is provider-supplied unique volume id.
	VolumeId string `yaml:"id" json:"id"`

	// from params.Volume
	HardwareId string `yaml:"hardwareid" json:"hardwareid"`

	// from params.Volume
	Size uint64 `yaml:"size" json:"size"`

	// from params.Volume
	Persistent bool `yaml:"persistent" json:"persistent"`

	// from params.VolumeAttachments
	DeviceName string `yaml:"device,omitempty" json:"device,omitempty"`

	// from params.VolumeAttachments
	ReadOnly bool `yaml:"read-only" json:"read-only"`

	// from params.Volume. This is juju volume id.
	Volume string `yaml:"volume,omitempty" json:"volume,omitempty"`

	// from params.Volume.
	Status EntityStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

type EntityStatus struct {
	Current params.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
}

// convertToVolumeInfo returns map of maps with volume info
// keyed first on machine ID and then on volume ID.
func convertToVolumeInfo(all []params.VolumeDetailsResult) (map[string]map[string]map[string]VolumeInfo, error) {
	result := map[string]map[string]map[string]VolumeInfo{}
	for _, one := range all {
		if err := convertVolumeDetailsResult(one, result); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

func convertVolumeDetailsResult(item params.VolumeDetailsResult, all map[string]map[string]map[string]VolumeInfo) error {
	info, attachments, storage, storageOwner := createVolumeInfo(item)
	for machineTag, attachmentInfo := range attachments {
		machineId, err := idFromTag(machineTag)
		if err != nil {
			return errors.Trace(err)
		}
		info.DeviceName = attachmentInfo.DeviceName
		info.ReadOnly = attachmentInfo.ReadOnly
		addOneToAll(machineId, storage, storageOwner, info, all)
	}
	if len(attachments) == 0 {
		addOneToAll("unattached", storage, storageOwner, info, all)
	}
	return nil
}

var idFromTag = func(s string) (string, error) {
	tag, err := names.ParseTag(s)
	if err != nil {
		return "", errors.Annotatef(err, "invalid tag %v", tag)
	}
	return tag.Id(), nil
}

func convertVolumeAttachments(item params.VolumeDetailsResult, all map[string]map[string]map[string]VolumeInfo) error {
	return nil
}

func addOneToAll(machineId, storageId, storageOwnerId string, item VolumeInfo, all map[string]map[string]map[string]VolumeInfo) {
	machineVolumes, ok := all[machineId]
	if !ok {
		machineVolumes = map[string]map[string]VolumeInfo{}
		all[machineId] = machineVolumes
	}
	storageOwnerVolumes, ok := machineVolumes[storageOwnerId]
	if !ok {
		storageOwnerVolumes = map[string]VolumeInfo{}
		machineVolumes[storageOwnerId] = storageOwnerVolumes
	}
	storageOwnerVolumes[storageId] = item
}

// TODO(axw) there are a lot of ignored errors in here, which are *probably*
// nil, but we should report them up the stack to be safe.
func createVolumeInfo(result params.VolumeDetailsResult) (
	info VolumeInfo,
	attachments map[string]params.VolumeAttachmentInfo,
	storageId string,
	storageOwnerId string,
) {
	details := result.Details
	if details == nil {
		details = volumeDetailsFromLegacy(result)
	}

	info.VolumeId = details.Info.VolumeId
	info.HardwareId = details.Info.HardwareId
	info.Size = details.Info.Size
	info.Persistent = details.Info.Persistent
	info.Status = EntityStatus{
		details.Status.Status,
		details.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(details.Status.Since, false),
	}
	if v, err := idFromTag(details.VolumeTag); err == nil {
		info.Volume = v
	}
	var err error
	if storageId, err = idFromTag(details.StorageTag); err != nil {
		// volume is not assigned to a storage instance
		storageId = "unassigned"
	}
	if storageOwnerId, err = idFromTag(details.StorageOwnerTag); err != nil {
		// volume is not assigned to a storage instance,
		// or storage instance is not attached to a unit (legacy)
		storageOwnerId = "unattached"
	}
	attachments = details.MachineAttachments
	return info, attachments, storageId, storageOwnerId
}

// volumeDetailsFromLegacy converts from legacy data structures
// to params.VolumeDetails. This exists only for backwards-
// compatibility. Please think long and hard before changing it.
func volumeDetailsFromLegacy(result params.VolumeDetailsResult) *params.VolumeDetails {
	details := &params.VolumeDetails{
		VolumeTag:  result.LegacyVolume.VolumeTag,
		StorageTag: result.LegacyVolume.StorageTag,
		Status:     result.LegacyVolume.Status,
	}
	details.Info.VolumeId = result.LegacyVolume.VolumeId
	details.Info.HardwareId = result.LegacyVolume.HardwareId
	details.Info.Size = result.LegacyVolume.Size
	details.Info.Persistent = result.LegacyVolume.Persistent
	if result.LegacyVolume.UnitTag != "" {
		details.StorageOwnerTag = result.LegacyVolume.UnitTag
	}
	if len(result.LegacyAttachments) > 0 {
		attachments := make(map[string]params.VolumeAttachmentInfo)
		for _, attachment := range result.LegacyAttachments {
			attachments[attachment.MachineTag] = attachment.Info
		}
		details.MachineAttachments = attachments
	}
	return details
}
