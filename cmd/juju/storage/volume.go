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
 Juju environment.
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
	Serial string `yaml:"serial" json:"serial"`
	// from params.Volume
	Size uint64 `yaml:"size" json:"size"`
	// from params.Volume
	Persistent bool `yaml:"persistent" json:"persistent"`
	// from params.VolumeAttachments
	DeviceName string `yaml:"device-name,omitempty" json:"device-name,omitempty"`
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
		addOneToAll("", item.Volume.VolumeId, unattached, all)
		return nil
	}
	// add info for volume attachments
	return convertVolumeAttachments(item, all)
}

func convertVolumeAttachments(item params.VolumeItem, all map[string]map[string]VolumeInfo) error {
	idFromTag := func(machine string) (string, error) {
		tag, err := names.ParseTag(machine)
		if err != nil {
			return "", errors.Annotatef(err, "invalid machine tag %v", machine)
		}
		return tag.Id(), nil
	}

	for _, one := range item.Attachments {
		machine, err := idFromTag(one.MachineTag)
		if err != nil {
			return errors.Trace(err)
		}
		info := createInfo(item.Volume)
		info.DeviceName = one.DeviceName
		info.ReadOnly = one.ReadOnly

		addOneToAll(machine, item.Volume.VolumeId, info, all)
	}
	return nil
}

func addOneToAll(machineId, volumeId string, item VolumeInfo, all map[string]map[string]VolumeInfo) {
	machineVolumes, ok := all[machineId]
	if !ok {
		machineVolumes = map[string]VolumeInfo{}
		all[machineId] = machineVolumes
	}
	machineVolumes[volumeId] = item
}

func createInfo(volume params.Volume) VolumeInfo {
	return VolumeInfo{
		Serial:     volume.Serial,
		Size:       volume.Size,
		Persistent: volume.Persistent,
	}
}
