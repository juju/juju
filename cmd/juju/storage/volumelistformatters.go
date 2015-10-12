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

// formatVolumeListTabular returns a tabular summary of volume instances.
func formatVolumeListTabular(value interface{}) ([]byte, error) {
	infos, ok := value.(map[string]VolumeInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", infos, value)
	}
	return formatVolumeListTabularTyped(infos), nil
}

func formatVolumeListTabularTyped(infos map[string]VolumeInfo) []byte {
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
	print("MACHINE", "UNIT", "STORAGE", "ID", "PROVIDER-ID", "DEVICE", "SIZE", "STATE", "MESSAGE")

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
			volumeAttachmentInfo.MachineVolumeAttachment = machineInfo
			for unitId, unitInfo := range info.Attachments.Units {
				if unitInfo.MachineId == machineId {
					volumeAttachmentInfo.UnitId = unitId
					volumeAttachmentInfo.UnitStorageAttachment = unitInfo
					break
				}
			}
			volumeAttachmentInfos = append(volumeAttachmentInfos, volumeAttachmentInfo)
		}
	}
	sort.Sort(volumeAttachmentInfos)

	for _, info := range volumeAttachmentInfos {
		var size string
		if info.Size > 0 {
			size = humanize.IBytes(info.Size * humanize.MiByte)
		}
		print(
			info.MachineId, info.UnitId, info.Storage,
			info.VolumeId, info.ProviderVolumeId,
			info.DeviceName, size,
			string(info.Status.Current), info.Status.Message,
		)
	}

	tw.Flush()
	return out.Bytes()
}

type volumeAttachmentInfo struct {
	VolumeId string
	VolumeInfo

	MachineId string
	MachineVolumeAttachment

	UnitId string
	UnitStorageAttachment
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
