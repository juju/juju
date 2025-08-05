// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/rpc/params"
)

// VolumeInfo defines the serialization behaviour for storage volume.
type VolumeInfo struct {
	// from params.Volume. This is provider-supplied unique volume ID.
	ProviderVolumeId string `yaml:"provider-id,omitempty" json:"provider-id,omitempty"`

	// Storage is the ID of the storage instance that the volume is
	// assigned to, if any.
	Storage string `yaml:"storage,omitempty" json:"storage,omitempty"`

	// Attachments is the set of entities attached to the volume.
	Attachments *VolumeAttachments `yaml:"attachments,omitempty" json:"attachments,omitempty"`

	// Pool is the name of the storage pool that the volume came from.
	Pool string `yaml:"pool,omitempty" json:"pool,omitempty"`

	// from params.Volume
	HardwareId string `yaml:"hardware-id,omitempty" json:"hardware-id,omitempty"`

	// from params.Volume
	WWN string `yaml:"wwn,omitempty" json:"wwn,omitempty"`

	// from params.Volume
	Size uint64 `yaml:"size" json:"size"`

	// from params.Volume
	Persistent bool `yaml:"persistent" json:"persistent"`

	// Life is the lifecycle state of the volume.
	Life string `yaml:"life,omitempty" json:"life,omitempty"`

	// from params.Volume
	Status EntityStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

type EntityStatus struct {
	Current status.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
}

type VolumeAttachments struct {
	Machines   map[string]VolumeAttachment      `yaml:"machines,omitempty" json:"machines,omitempty"`
	Containers map[string]VolumeAttachment      `yaml:"containers,omitempty" json:"containers,omitempty"`
	Units      map[string]UnitStorageAttachment `yaml:"units,omitempty" json:"units,omitempty"`
}

type VolumeAttachment struct {
	DeviceName string `yaml:"device,omitempty" json:"device,omitempty"`
	DeviceLink string `yaml:"device-link,omitempty" json:"device-link,omitempty"`
	BusAddress string `yaml:"bus-address,omitempty" json:"bus-address,omitempty"`
	ReadOnly   bool   `yaml:"read-only" json:"read-only"`
	Life       string `yaml:"life,omitempty" json:"life,omitempty"`
	// TODO(axw) add machine volume attachment status when we have it
}

// generateListVolumeOutput returns a map of volume info
func generateListVolumeOutput(ctx *cmd.Context, api StorageListAPI, ids []string) (map[string]VolumeInfo, error) {

	results, err := api.ListVolumes(ctx, ids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// filter out valid output, if any
	var valid []params.VolumeDetails
	for _, result := range results {
		if result.Error == nil {
			valid = append(valid, result.Result...)
			continue
		}
		// display individual error
		fmt.Fprintf(ctx.Stderr, "%v\n", result.Error)
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return convertToVolumeInfo(valid)
}

// convertToVolumeInfo returns a map of volume IDs to volume info.
func convertToVolumeInfo(all []params.VolumeDetails) (map[string]VolumeInfo, error) {
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

func createVolumeInfo(details params.VolumeDetails) (names.VolumeTag, VolumeInfo, error) {
	volumeTag, err := names.ParseVolumeTag(details.VolumeTag)
	if err != nil {
		return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
	}

	var info VolumeInfo
	info.ProviderVolumeId = details.Info.ProviderId
	info.HardwareId = details.Info.HardwareId
	info.WWN = details.Info.WWN
	info.Pool = details.Info.Pool
	info.Size = details.Info.SizeMiB
	info.Persistent = details.Info.Persistent
	info.Life = string(details.Life)
	info.Status = EntityStatus{
		details.Status.Status,
		details.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(details.Status.Since, false),
	}

	attachmentsFromDetails := func(
		in map[string]params.VolumeAttachmentDetails,
		out map[string]VolumeAttachment,
	) error {
		for tag, attachment := range in {
			id, err := idFromTag(tag)
			if err != nil {
				return errors.Trace(err)
			}
			out[id] = VolumeAttachment{
				attachment.DeviceName,
				attachment.DeviceLink,
				attachment.BusAddress,
				attachment.ReadOnly,
				string(attachment.Life),
			}
		}
		return nil
	}

	if len(details.MachineAttachments) > 0 {
		machineAttachments := make(map[string]VolumeAttachment)
		if err := attachmentsFromDetails(details.MachineAttachments, machineAttachments); err != nil {
			return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
		}
		info.Attachments = &VolumeAttachments{
			Machines: machineAttachments,
		}
	}

	if len(details.UnitAttachments) > 0 {
		unitAttachments := make(map[string]VolumeAttachment)
		if err := attachmentsFromDetails(details.UnitAttachments, unitAttachments); err != nil {
			return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
		}
		if info.Attachments == nil {
			info.Attachments = &VolumeAttachments{}
		}
		info.Attachments.Containers = unitAttachments
	}

	if details.Storage != nil {
		storageTag, storageInfo, err := createStorageInfo(*details.Storage)
		if err != nil {
			return names.VolumeTag{}, VolumeInfo{}, errors.Trace(err)
		}
		info.Storage = storageTag.Id()
		if storageInfo.Attachments != nil {
			if info.Attachments == nil {
				info.Attachments = &VolumeAttachments{}
			}
			info.Attachments.Units = storageInfo.Attachments.Units
		}
	}

	return volumeTag, info, nil
}
