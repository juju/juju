// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/juju/ansiterm"

	"github.com/juju/juju/cmd/output"
)

// formatStorageInstancesListTabular writes a tabular summary of storage instances.
func formatStorageInstancesListTabular(writer io.Writer, s CombinedStorage) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}

	storagePool, storageSize := getStoragePoolAndSize(s)
	units, byUnit := sortStorageInstancesByUnitId(s)

	w.Print("Unit", "Storage ID", "Type")
	if len(storagePool) > 0 {
		// Older versions of Juju do not include
		// the pool name in the storage details.
		// We omit the column in that case.
		w.Print("Pool")
	}
	w.Println("Size", "Status", "Message")

	for _, unit := range units {
		// Then sort by storage IDs
		byStorage := byUnit[unit]
		storageIds := make([]string, 0, len(byStorage))
		for storageId := range byStorage {
			storageIds = append(storageIds, storageId)
		}
		sort.Strings(slashSeparatedIds(storageIds))

		for _, storageId := range storageIds {
			info := byStorage[storageId]
			w.Print(info.unitId)
			w.Print(info.storageId)
			w.Print(info.kind)
			if len(storagePool) > 0 {
				w.Print(storagePool[info.storageId])
			}
			w.Print(humanizeStorageSize(storageSize[storageId]))
			w.PrintStatus(info.status.Current)
			w.Println(info.status.Message)
		}
	}
	return tw.Flush()
}

func sortStorageInstancesByUnitId(s CombinedStorage) ([]string, map[string]map[string]storageAttachmentInfo) {
	byUnit := make(map[string]map[string]storageAttachmentInfo)
	for storageId, storageInfo := range s.StorageInstances {
		if storageInfo.Attachments == nil {
			byStorage := byUnit[""]
			if byStorage == nil {
				byStorage = make(map[string]storageAttachmentInfo)
				byUnit[""] = byStorage
			}
			byStorage[storageId] = storageAttachmentInfo{
				storageId: storageId,
				kind:      storageInfo.Kind,
				status:    storageInfo.Status,
			}
			continue
		}
		for unitId := range storageInfo.Attachments.Units {
			byStorage := byUnit[unitId]
			if byStorage == nil {
				byStorage = make(map[string]storageAttachmentInfo)
				byUnit[unitId] = byStorage
			}
			byStorage[storageId] = storageAttachmentInfo{
				storageId: storageId,
				unitId:    unitId,
				kind:      storageInfo.Kind,
				status:    storageInfo.Status,
			}
		}
	}

	// sort by units
	units := make([]string, 0, len(s.StorageInstances))
	for unit := range byUnit {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units, byUnit
}

func getStoragePoolAndSize(s CombinedStorage) (map[string]string, map[string]uint64) {
	storageSize := make(map[string]uint64)
	storagePool := make(map[string]string)
	for _, f := range s.Filesystems {
		if f.Pool != "" {
			storagePool[f.Storage] = f.Pool
		}
		storageSize[f.Storage] = f.Size
	}
	for _, v := range s.Volumes {
		// This will intentionally override the provider ID
		// and pool for a volume-backed filesystem.
		if v.Pool != "" {
			storagePool[v.Storage] = v.Pool
		}
		// For size, we want to use the size of the filesystem
		// rather than the volume.
		if _, ok := storageSize[v.Storage]; !ok {
			storageSize[v.Storage] = v.Size
		}
	}
	return storagePool, storageSize
}

func getFilesystemAttachment(combined CombinedStorage, attachmentInfo storageAttachmentInfo) FilesystemAttachment {
	for _, f := range combined.Filesystems {
		if f.Storage == attachmentInfo.storageId {
			infos, _ := extractFilesystemAttachmentInfo(filesystemAttachmentInfos{}, "", f)
			for _, info := range infos {
				if info.UnitId == attachmentInfo.unitId {
					return info.FilesystemAttachment
				}
			}
		}
	}
	return FilesystemAttachment{}
}

// FormatStorageListForStatusTabular writes a tabular summary of storage for status tabular view.
func FormatStorageListForStatusTabular(writer *ansiterm.TabWriter, s CombinedStorage) error {
	w := output.Wrapper{writer}

	storagePool, storageSize := getStoragePoolAndSize(s)
	units, byUnit := sortStorageInstancesByUnitId(s)

	w.Println()
	w.Print("Storage Unit", "Storage ID", "Type")
	if len(storagePool) > 0 {
		w.Print("Pool")
	}
	w.Println("Mountpoint", "Size", "Status", "Message")

	for _, unit := range units {
		byStorage := byUnit[unit]
		storageIds := make([]string, 0, len(byStorage))
		for storageId := range byStorage {
			storageIds = append(storageIds, storageId)
		}
		sort.Strings(storageIds)

		for _, storageId := range storageIds {
			info := byStorage[storageId]

			w.Print(info.unitId)
			w.Print(info.storageId)
			w.Print(info.kind)
			if len(storagePool) > 0 {
				w.Print(storagePool[info.storageId])
			}
			w.Print(getFilesystemAttachment(s, info).MountPoint)
			w.Print(humanizeStorageSize(storageSize[storageId]))
			w.PrintStatus(info.status.Current)
			w.PrintColorNoTab(output.EmphasisHighlight.Gray, info.status.Message)
			w.Println()
		}
	}
	return w.Flush()
}

func humanizeStorageSize(size uint64) string {
	var sizeStr string
	if size > 0 {
		sizeStr = humanize.IBytes(size * humanize.MiByte)
	}
	return sizeStr
}

type storageAttachmentInfo struct {
	storageId string
	unitId    string
	kind      string
	status    EntityStatus
}

// slashSeparatedIds represents a list of slash separated ids.
type slashSeparatedIds []string

func (s slashSeparatedIds) Len() int {
	return len(s)
}

func (s slashSeparatedIds) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s slashSeparatedIds) Less(a, b int) bool {
	return compareSlashSeparated(s[a], s[b]) == -1
}

// compareSlashSeparated compares a with b, first the string before
// "/", and then the integer or string after. Empty strings are sorted
// after all others.
func compareSlashSeparated(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	sa := strings.SplitN(a, "/", 2)
	sb := strings.SplitN(b, "/", 2)
	if sa[0] < sb[0] {
		return -1
	}
	if sa[0] > sb[0] {
		return 1
	}

	getInt := func(suffix string) (bool, int) {
		num, err := strconv.Atoi(suffix)
		if err != nil {
			return false, 0
		}
		return true, num
	}

	naIsNumeric, na := getInt(sa[1])
	if !naIsNumeric {
		return compareStrings(sa[1], sb[1])
	}
	nbIsNumeric, nb := getInt(sb[1])
	if !nbIsNumeric {
		return compareStrings(sa[1], sb[1])
	}

	switch {
	case na < nb:
		return -1
	case na == nb:
		return 0
	}
	return 1
}

// compareStrings does what strings.Compare does, but without using
// strings.Compare as it does not exist in Go 1.2.
func compareStrings(a, b string) int {
	if a == b {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}
