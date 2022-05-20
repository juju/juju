// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/juju/juju/cmd/output"
)

// formatVolumeListTabular returns a tabular summary of volume instances.
func formatVolumeListTabular(writer io.Writer, infos map[string]VolumeInfo) error {
	tw := output.TabWriter(writer)

	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	haveMachines := false
	volumeAttachmentInfos := make(volumeAttachmentInfos, 0, len(infos))
	for volumeId, info := range infos {
		volumeAttachmentInfo := volumeAttachmentInfo{
			VolumeId:   volumeId,
			VolumeInfo: info,
		}
		if info.Attachments == nil {
			volumeAttachmentInfos = append(volumeAttachmentInfos, volumeAttachmentInfo)
			continue
		}
		// Each unit attachment must have a corresponding volume
		// attachment. Enumerate each of the volume attachments,
		// and locate the corresponding unit attachment if any.
		// Each volume attachment has at most one corresponding
		// unit attachment.
		for machineId, machineInfo := range info.Attachments.Machines {
			volumeAttachmentInfo := volumeAttachmentInfo
			volumeAttachmentInfo.MachineId = machineId
			volumeAttachmentInfo.VolumeAttachment = machineInfo
			for unitId, unitInfo := range info.Attachments.Units {
				if unitInfo.MachineId == machineId {
					volumeAttachmentInfo.UnitId = unitId
					volumeAttachmentInfo.UnitStorageAttachment = unitInfo
					break
				}
			}
			haveMachines = true
			volumeAttachmentInfos = append(volumeAttachmentInfos, volumeAttachmentInfo)
		}

		for hostId, containerInfo := range info.Attachments.Containers {
			volumeAttachmentInfo := volumeAttachmentInfo
			volumeAttachmentInfo.VolumeAttachment = containerInfo
			for unitId, unitInfo := range info.Attachments.Units {
				if hostId == unitId {
					volumeAttachmentInfo.UnitId = unitId
					volumeAttachmentInfo.UnitStorageAttachment = unitInfo
					break
				}
			}
			volumeAttachmentInfos = append(volumeAttachmentInfos, volumeAttachmentInfo)
		}
	}
	sort.Sort(volumeAttachmentInfos)

	if haveMachines {
		print("Machine", "Unit", "Storage ID", "Volume ID", "Provider ID", "Device", "Size", "State", "Message")
	} else {
		print("Unit", "Storage ID", "Volume ID", "Provider ID", "Size", "State", "Message")
	}

	for _, info := range volumeAttachmentInfos {
		var size string
		if info.Size > 0 {
			size = humanize.IBytes(info.Size * humanize.MiByte)
		}
		if haveMachines {
			print(
				info.MachineId, info.UnitId, info.Storage,
				info.VolumeId, info.ProviderVolumeId,
				info.VolumeAttachment.DeviceName, size,
				string(info.Status.Current), info.Status.Message,
			)
		} else {
			print(
				info.UnitId, info.Storage,
				info.VolumeId, info.ProviderVolumeId, size,
				string(info.Status.Current), info.Status.Message,
			)
		}
	}

	return tw.Flush()
}

type volumeAttachmentInfo struct {
	VolumeId string
	VolumeInfo

	MachineId        string
	VolumeAttachment VolumeAttachment

	UnitId                string
	UnitStorageAttachment UnitStorageAttachment
}

type volumeAttachmentInfos []volumeAttachmentInfo

func (v volumeAttachmentInfos) Len() int {
	return len(v)
}

func (v volumeAttachmentInfos) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v volumeAttachmentInfos) Less(i, j int) bool {
	switch compareStrings(v[i].MachineId, v[j].MachineId) {
	case -1:
		return true
	case 1:
		return false
	}

	switch compareSlashSeparated(v[i].UnitId, v[j].UnitId) {
	case -1:
		return true
	case 1:
		return false
	}

	switch compareSlashSeparated(v[i].Storage, v[j].Storage) {
	case -1:
		return true
	case 1:
		return false
	}

	return v[i].VolumeId < v[j].VolumeId
}
