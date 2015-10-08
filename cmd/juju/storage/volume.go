// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
)

const volumeCmdDoc = `
"juju storage volume" is used to manage storage volumes in
 the Juju environment.
`

const volumeCmdPurpose = "manage storage volumes"

// newVolumeSuperCommand creates the storage volume super subcommand and
// registers the subcommands that it supports.
func newVolumeSuperCommand() cmd.Command {
	supercmd := jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
		Name:        "volume",
		Doc:         volumeCmdDoc,
		UsagePrefix: "juju storage",
		Purpose:     volumeCmdPurpose,
	})
	supercmd.Register(newVolumeListCommand())
	return supercmd
}

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}

// VolumeInfo defines the serialization behaviour for storage volume.
type VolumeInfo struct {
	// from params.Volume. This is provider-supplied unique volume id.
	ProviderVolumeId string `yaml:"provider-id,omitempty" json:"provider-id,omitempty"`

	// Storage is the ID of the storage instance that the volume is
	// assigned to, if any.
	Storage string `yaml:"storage,omitempty" json:"storage,omitempty"`

	// Attachments is the set of entities attached to the volume.
	Attachments *VolumeAttachments `yaml:"attachments,omitempty" json:"attachments,omitempty"`

	// from params.Volume
	HardwareId string `yaml:"hardware-id,omitempty" json:"hardware-id,omitempty"`

	// from params.Volume
	Size uint64 `yaml:"size" json:"size"`

	// from params.Volume
	Persistent bool `yaml:"persistent" json:"persistent"`

	// from params.Volume
	Status EntityStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

type EntityStatus struct {
	Current params.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
}

type VolumeAttachments struct {
	Machines map[string]MachineVolumeAttachment `yaml:"machines,omitempty" json:"machines,omitempty"`
	Units    map[string]UnitStorageAttachment   `yaml:"units,omitempty" json:"units,omitempty"`
}

type MachineVolumeAttachment struct {
	DeviceName string `yaml:"device,omitempty" json:"device,omitempty"`
	DeviceLink string `yaml:"device-link,omitempty" json:"device-link,omitempty"`
	BusAddress string `yaml:"bus-address,omitempty" json:"bus-address,omitempty"`
	ReadOnly   bool   `yaml:"read-only" json:"read-only"`
	// TODO(axw) add machine volume attachment status when we have it
}

// convertToVolumeInfo returns a map of volume IDs to volume info.
func convertToVolumeInfo(all []params.VolumeDetailsResult) (map[string]VolumeInfo, error) {
	result := make(map[string]VolumeInfo)
	for _, one := range all {
		volumeTag, info, err := createVolumeInfo(one)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[volumeTag.Id()] = info
	}
	return result, nil
}

var idFromTag = func(s string) (string, error) {
	tag, err := names.ParseTag(s)
	if err != nil {
		return "", errors.Annotatef(err, "invalid tag %v", tag)
	}
	return tag.Id(), nil
}

func createVolumeInfo(result params.VolumeDetailsResult) (names.VolumeTag, VolumeInfo, error) {
	details := result.Details
	if details == nil {
		details = volumeDetailsFromLegacy(result)
	}

	volumeTag, err := names.ParseVolumeTag(details.VolumeTag)
	if err != nil {
		return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
	}

	var info VolumeInfo
	info.ProviderVolumeId = details.Info.VolumeId
	info.HardwareId = details.Info.HardwareId
	info.Size = details.Info.Size
	info.Persistent = details.Info.Persistent
	info.Status = EntityStatus{
		details.Status.Status,
		details.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(details.Status.Since, false),
	}

	if len(details.MachineAttachments) > 0 {
		machineAttachments := make(map[string]MachineVolumeAttachment)
		for machineTag, attachment := range details.MachineAttachments {
			machineId, err := idFromTag(machineTag)
			if err != nil {
				return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
			}
			machineAttachments[machineId] = MachineVolumeAttachment{
				attachment.DeviceName,
				attachment.DeviceLink,
				attachment.BusAddress,
				attachment.ReadOnly,
			}
		}
		info.Attachments = &VolumeAttachments{
			Machines: machineAttachments,
		}
	}

	if details.Storage != nil {
		storageTag, storageInfo, err := createStorageInfo(*details.Storage)
		if err != nil {
			return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
		}
		info.Storage = storageTag.Id()
		if storageInfo.Attachments != nil {
			info.Attachments.Units = storageInfo.Attachments.Units
		}
	}

	return volumeTag, info, nil
}

// volumeDetailsFromLegacy converts from legacy data structures
// to params.VolumeDetails. This exists only for backwards-
// compatibility. Please think long and hard before changing it.
func volumeDetailsFromLegacy(result params.VolumeDetailsResult) *params.VolumeDetails {
	details := &params.VolumeDetails{
		VolumeTag: result.LegacyVolume.VolumeTag,
		Status:    result.LegacyVolume.Status,
	}
	details.Info.VolumeId = result.LegacyVolume.VolumeId
	details.Info.HardwareId = result.LegacyVolume.HardwareId
	details.Info.Size = result.LegacyVolume.Size
	details.Info.Persistent = result.LegacyVolume.Persistent
	if len(result.LegacyAttachments) > 0 {
		attachments := make(map[string]params.VolumeAttachmentInfo)
		for _, attachment := range result.LegacyAttachments {
			attachments[attachment.MachineTag] = attachment.Info
		}
		details.MachineAttachments = attachments
	}
	if result.LegacyVolume.StorageTag != "" {
		details.Storage = &params.StorageDetails{
			StorageTag: result.LegacyVolume.StorageTag,
			Status:     details.Status,
		}
		if result.LegacyVolume.UnitTag != "" {
			// Servers with legacy storage do not support shared
			// storage, so there will only be one attachment, and
			// the owner is always a unit.
			details.Storage.OwnerTag = result.LegacyVolume.UnitTag
			if len(result.LegacyAttachments) == 1 {
				details.Storage.Attachments = map[string]params.StorageAttachmentDetails{
					result.LegacyVolume.UnitTag: params.StorageAttachmentDetails{
						StorageTag: result.LegacyVolume.StorageTag,
						UnitTag:    result.LegacyVolume.UnitTag,
						MachineTag: result.LegacyAttachments[0].MachineTag,
						// Don't set Location, because we can't infer that
						// from the legacy volume details.
						Location: "",
					},
				}
			}
		}
	}
	return details
}
