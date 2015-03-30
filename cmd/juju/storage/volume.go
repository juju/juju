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
)

const volumeCmdDoc = `
"juju storage volume" is used to manage storage volumes in
 the Juju environment.
`

const volumeCmdPurpose = "manage storage volumes"

// NewVolumeSuperCommand creates the storage volume super subcommand and
// registers the subcommands that it supports.
func NewVolumeSuperCommand() cmd.Command {
	poolcmd := Command{
		SuperCommand: *jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
			Name:        "volume",
			Doc:         volumeCmdDoc,
			UsagePrefix: "juju storage",
			Purpose:     volumeCmdPurpose,
		})}
	poolcmd.Register(envcmd.Wrap(&VolumeListCommand{}))
	return &poolcmd
}

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}

// VolumeInfo defines the serialization behaviour for storage volume.
type VolumeInfo struct {
	// from params.Volume
	VolumeId string `yaml:"id" json:"id"`

	// from params.Volume
	StorageId string `yaml:"storage" json:"storage"`

	// from params.Volume
	Serial string `yaml:"serial" json:"serial"`

	// from params.Volume
	Size uint64 `yaml:"size" json:"size"`

	// from params.Volume
	Persistent bool `yaml:"persistent" json:"persistent"`

	// from params.VolumeAttachments
	DeviceName string `yaml:"device,omitempty" json:"device,omitempty"`

	// from params.VolumeAttachments
	ReadOnly bool `yaml:"readonly" json:"readonly"`
}

// convertToVolumeInfo returns map of maps with volume info
// keyed first on machine_id and then on volume_id.
func convertToVolumeInfo(all []params.VolumeItem) (map[string]map[string]VolumeInfo, error) {
	result := map[string]map[string]VolumeInfo{}
	for _, one := range all {
		if err := convertVolumeItem(one, result); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

func convertVolumeItem(item params.VolumeItem, all map[string]map[string]VolumeInfo) error {
	if len(item.Attachments) == 0 {
		unattached := createInfo(item.Volume)
		err := addOneToAll("unattached", item.Volume.VolumeTag, unattached, all)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	// add info for volume attachments
	return convertVolumeAttachments(item, all)
}

var idFromTag = func(s string) (string, error) {
	tag, err := names.ParseTag(s)
	if err != nil {
		return "", errors.Annotatef(err, "invalid tag %v", tag)
	}
	return tag.Id(), nil
}

func convertVolumeAttachments(item params.VolumeItem, all map[string]map[string]VolumeInfo) error {
	for _, one := range item.Attachments {
		machine, err := idFromTag(one.MachineTag)
		if err != nil {
			return errors.Trace(err)
		}
		info := createInfo(item.Volume)
		info.DeviceName = one.DeviceName
		info.ReadOnly = one.ReadOnly

		err = addOneToAll(machine, item.Volume.VolumeTag, info, all)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func addOneToAll(machineId, volumeTag string, item VolumeInfo, all map[string]map[string]VolumeInfo) error {
	machineVolumes, ok := all[machineId]
	if !ok {
		machineVolumes = map[string]VolumeInfo{}
		all[machineId] = machineVolumes
	}
	volume, err := idFromTag(volumeTag)
	if err != nil {
		return errors.Trace(err)
	}

	machineVolumes[volume] = item
	return nil
}

func createInfo(volume params.VolumeInstance) VolumeInfo {
	result := VolumeInfo{
		VolumeId:   volume.VolumeId,
		Serial:     volume.Serial,
		Size:       volume.Size,
		Persistent: volume.Persistent,
	}
	if storage, err := idFromTag(volume.StorageTag); err == nil {
		result.StorageId = storage
	}
	return result
}
