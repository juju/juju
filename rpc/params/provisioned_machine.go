// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/domain/provisioner"
)

// ProvisionedMachineInfo converts the successful provisioning result carried
// by InstanceInfo to the domain representation. Volume tags are reduced to the
// identifiers used by the storage provisioning service.
func (arg InstanceInfo) ProvisionedMachineInfo() (provisioner.ProvisionedMachineInfo, error) {
	info := provisioner.ProvisionedMachineInfo{
		InstanceID:              arg.InstanceId,
		DisplayName:             arg.DisplayName,
		Nonce:                   arg.Nonce,
		HardwareCharacteristics: arg.Characteristics,
		NetworkConfig:           InterfaceInfoFromNetworkConfig(arg.NetworkConfig),
		Volumes:                 make([]provisioner.ProvisionedVolume, 0, len(arg.Volumes)),
		VolumeAttachments:       make(map[string]provisioner.ProvisionedVolumeAttachment, len(arg.VolumeAttachments)),
	}
	for _, volume := range arg.Volumes {
		tag, err := names.ParseVolumeTag(volume.VolumeTag)
		if err != nil {
			return provisioner.ProvisionedMachineInfo{}, err
		}
		info.Volumes = append(info.Volumes, provisioner.ProvisionedVolume{
			VolumeID:   tag.Id(),
			ProviderID: volume.Info.ProviderId,
			HardwareID: volume.Info.HardwareId,
			WWN:        volume.Info.WWN,
			SizeMiB:    volume.Info.SizeMiB,
			Persistent: volume.Info.Persistent,
		})
	}
	for volumeTag, attachment := range arg.VolumeAttachments {
		tag, err := names.ParseVolumeTag(volumeTag)
		if err != nil {
			return provisioner.ProvisionedMachineInfo{}, err
		}
		provisionedAttachment := provisioner.ProvisionedVolumeAttachment{
			DeviceName: attachment.DeviceName,
			DeviceLink: attachment.DeviceLink,
			BusAddress: attachment.BusAddress,
			ReadOnly:   attachment.ReadOnly,
		}
		if attachment.PlanInfo != nil {
			provisionedAttachment.Plan = &provisioner.ProvisionedVolumeAttachmentPlan{
				DeviceType:       attachment.PlanInfo.DeviceType,
				DeviceAttributes: attachment.PlanInfo.DeviceAttributes,
			}
		}
		info.VolumeAttachments[tag.Id()] = provisionedAttachment
	}
	return info, nil
}
