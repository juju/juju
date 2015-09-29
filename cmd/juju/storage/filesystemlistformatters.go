// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
)

// formatFilesystemListTabular returns a tabular summary of filesystem instances.
func formatFilesystemListTabular(value interface{}) ([]byte, error) {
	infos, ok := value.(map[string]FilesystemInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", infos, value)
	}
	return formatFilesystemListTabularTyped(infos), nil
}

func formatFilesystemListTabularTyped(infos map[string]FilesystemInfo) []byte {
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)

	print := func(values ...string) {
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}
	print("MACHINE", "UNIT", "STORAGE", "ID", "VOLUME", "PROVIDER-ID", "MOUNTPOINT", "SIZE", "STATE", "MESSAGE")

	filesystemAttachmentInfos := make(filesystemAttachmentInfos, 0, len(infos))
	for filesystemId, info := range infos {
		filesystemAttachmentInfo := filesystemAttachmentInfo{
			FilesystemId:   filesystemId,
			FilesystemInfo: info,
		}
		if info.Attachments == nil {
			filesystemAttachmentInfos = append(filesystemAttachmentInfos, filesystemAttachmentInfo)
			continue
		}
		// Each unit attachment must have a corresponding filesystem
		// attachment. Enumerate each of the filesystem attachments,
		// and locate the corresponding unit attachment if any.
		// Each filesystem attachment has at most one corresponding
		// unit attachment.
		for machineId, machineInfo := range info.Attachments.Machines {
			filesystemAttachmentInfo := filesystemAttachmentInfo
			filesystemAttachmentInfo.MachineId = machineId
			filesystemAttachmentInfo.MachineFilesystemAttachment = machineInfo
			for unitId, unitInfo := range info.Attachments.Units {
				if unitInfo.MachineId == machineId {
					filesystemAttachmentInfo.UnitId = unitId
					filesystemAttachmentInfo.UnitStorageAttachment = unitInfo
					break
				}
			}
			filesystemAttachmentInfos = append(filesystemAttachmentInfos, filesystemAttachmentInfo)
		}
	}
	sort.Sort(filesystemAttachmentInfos)

	for _, info := range filesystemAttachmentInfos {
		var size string
		if info.Size > 0 {
			size = humanize.IBytes(info.Size * humanize.MiByte)
		}
		print(
			info.MachineId, info.UnitId, info.Storage,
			info.FilesystemId, info.Volume, info.ProviderFilesystemId,
			info.MountPoint, size,
			string(info.Status.Current), info.Status.Message,
		)
	}

	tw.Flush()
	return out.Bytes()
}

type filesystemAttachmentInfo struct {
	FilesystemId string
	FilesystemInfo

	MachineId string
	MachineFilesystemAttachment

	UnitId string
	UnitStorageAttachment
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
