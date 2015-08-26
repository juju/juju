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
// keyed first on machine_id and then on volume_id.
func convertToVolumeInfo(all []params.VolumeItem) (map[string]map[string]map[string]VolumeInfo, error) {
	result := map[string]map[string]map[string]VolumeInfo{}
	for _, one := range all {
		if err := convertVolumeItem(one, result); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

func convertVolumeItem(item params.VolumeItem, all map[string]map[string]map[string]VolumeInfo) error {
	if len(item.Attachments) != 0 {
		// add info for volume attachments
		return convertVolumeAttachments(item, all)
	}
	unattached, unit, storage := createInfo(item.Volume)
	addOneToAll("unattached", unit, storage, unattached, all)
	return nil
}

var idFromTag = func(s string) (string, error) {
	tag, err := names.ParseTag(s)
	if err != nil {
		return "", errors.Annotatef(err, "invalid tag %v", tag)
	}
	return tag.Id(), nil
}

func convertVolumeAttachments(item params.VolumeItem, all map[string]map[string]map[string]VolumeInfo) error {
	for _, one := range item.Attachments {
		machine, err := idFromTag(one.MachineTag)
		if err != nil {
			return errors.Trace(err)
		}
		info, unit, storage := createInfo(item.Volume)
		info.DeviceName = one.Info.DeviceName
		info.ReadOnly = one.Info.ReadOnly

		addOneToAll(machine, unit, storage, info, all)
	}
	return nil
}

func addOneToAll(machineId, unitId, storageId string, item VolumeInfo, all map[string]map[string]map[string]VolumeInfo) {
	machineVolumes, ok := all[machineId]
	if !ok {
		machineVolumes = map[string]map[string]VolumeInfo{}
		all[machineId] = machineVolumes
	}
	unitVolumes, ok := machineVolumes[unitId]
	if !ok {
		unitVolumes = map[string]VolumeInfo{}
		machineVolumes[unitId] = unitVolumes
	}
	unitVolumes[storageId] = item
}

func createInfo(volume params.VolumeInstance) (info VolumeInfo, unit, storage string) {
	info.VolumeId = volume.VolumeId
	info.HardwareId = volume.HardwareId
	info.Size = volume.Size
	info.Persistent = volume.Persistent
	info.Status = EntityStatus{
		volume.Status.Status,
		volume.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(volume.Status.Since, false),
	}

	if v, err := idFromTag(volume.VolumeTag); err == nil {
		info.Volume = v
	}
	var err error
	if storage, err = idFromTag(volume.StorageTag); err != nil {
		storage = "unassigned"
	}
	if unit, err = idFromTag(volume.UnitTag); err != nil {
		unit = "unattached"
	}
	return
}
