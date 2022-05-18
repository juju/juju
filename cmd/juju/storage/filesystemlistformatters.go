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

// formatFilesystemListTabular writes a tabular summary of filesystem instances.
func formatFilesystemListTabular(writer io.Writer, infos map[string]FilesystemInfo) error {
	tw := output.TabWriter(writer)

	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	haveMachines := false
	var filesystemAttachmentInfos filesystemAttachmentInfos
	for filesystemId, info := range infos {
		var withMachines bool
		filesystemAttachmentInfos, withMachines = extractFilesystemAttachmentInfo(filesystemAttachmentInfos, filesystemId, info)
		haveMachines = haveMachines || withMachines
	}
	sort.Sort(filesystemAttachmentInfos)

	if haveMachines {
		print("Machine", "Unit", "Storage ID", "ID", "Volume", "Provider ID", "Mountpoint", "Size", "State", "Message")
	} else {
		print("Unit", "Storage ID", "ID", "Provider ID", "Mountpoint", "Size", "State", "Message")
	}

	for _, info := range filesystemAttachmentInfos {
		var size string
		if info.Size > 0 {
			size = humanize.IBytes(info.Size * humanize.MiByte)
		}
		if haveMachines {
			print(
				info.MachineId, info.UnitId, info.Storage,
				info.FilesystemId, info.Volume, info.ProviderFilesystemId,
				info.FilesystemAttachment.MountPoint, size,
				string(info.Status.Current), info.Status.Message,
			)
		} else {
			print(
				info.UnitId, info.Storage,
				info.FilesystemId, info.ProviderFilesystemId,
				info.FilesystemAttachment.MountPoint, size,
				string(info.Status.Current), info.Status.Message,
			)
		}
	}

	return tw.Flush()
}

func extractFilesystemAttachmentInfo(infos filesystemAttachmentInfos, fileSystemId string, fsInfo FilesystemInfo) (filesystemAttachmentInfos, bool) {
	// Each unit attachment must have a corresponding filesystem
	// attachment. Enumerate each of the filesystem attachments,
	// and locate the corresponding unit attachment if any.
	// Each filesystem attachment has at most one corresponding
	// unit attachment.
	if fsInfo.Attachments == nil {
		return infos, false
	}
	haveMachines := false
	info := filesystemAttachmentInfo{
		FilesystemId:   fileSystemId,
		FilesystemInfo: fsInfo,
	}
	for machineId, machineInfo := range fsInfo.Attachments.Machines {
		info.MachineId = machineId
		info.FilesystemAttachment = machineInfo
		for unitId, unitInfo := range info.Attachments.Units {
			if unitInfo.MachineId == machineId {
				info.UnitId = unitId
				info.UnitStorageAttachment = unitInfo
				break
			}
		}
		infos = append(infos, info)
		haveMachines = true
	}

	for hostId, containerInfo := range fsInfo.Attachments.Containers {
		info.FilesystemAttachment = containerInfo
		for unitId, unitInfo := range info.Attachments.Units {
			if hostId == unitId {
				info.UnitId = unitId
				info.UnitStorageAttachment = unitInfo
				break
			}
		}
		infos = append(infos, info)
	}
	return infos, haveMachines
}

type filesystemAttachmentInfo struct {
	FilesystemId string
	FilesystemInfo

	MachineId            string
	FilesystemAttachment FilesystemAttachment

	UnitId                string
	UnitStorageAttachment UnitStorageAttachment
}

type filesystemAttachmentInfos []filesystemAttachmentInfo

func (v filesystemAttachmentInfos) Len() int {
	return len(v)
}

func (v filesystemAttachmentInfos) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

func (v filesystemAttachmentInfos) Less(i, j int) bool {
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

	return v[i].FilesystemId < v[j].FilesystemId
}
