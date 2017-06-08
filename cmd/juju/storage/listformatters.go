// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/juju/juju/cmd/output"
)

// formatListTabular writes a tabular summary of storage instances.
func formatStorageListTabular(
	writer io.Writer,
	storageInfo map[string]StorageInfo,
	filesystems map[string]FilesystemInfo,
	volumes map[string]VolumeInfo,
) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println("[Storage]")

	storageProviderId := make(map[string]string)
	storageSize := make(map[string]uint64)
	storagePool := make(map[string]string)
	for _, f := range filesystems {
		if f.Pool != "" {
			storagePool[f.Storage] = f.Pool
		}
		storageProviderId[f.Storage] = f.ProviderFilesystemId
		storageSize[f.Storage] = f.Size
	}
	for _, v := range volumes {
		// This will intentionally override the provider ID
		// and pool for a volume-backed filesystem.
		if v.Pool != "" {
			storagePool[v.Storage] = v.Pool
		}
		storageProviderId[v.Storage] = v.ProviderVolumeId
		// For size, we want to use the size of the fileystem
		// rather than the volume.
		if _, ok := storageSize[v.Storage]; !ok {
			storageSize[v.Storage] = v.Size
		}
	}

	w.Print("Unit", "Id", "Type")
	if len(storagePool) > 0 {
		// Older versions of Juju do not include
		// the pool name in the storage details.
		// We omit the column in that case.
		w.Print("Pool")
	}
	w.Println("Provider id", "Size", "Status", "Message")

	byUnit := make(map[string]map[string]storageAttachmentInfo)
	for storageId, storageInfo := range storageInfo {
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

	// First sort by units
	units := make([]string, 0, len(storageInfo))
	for unit := range byUnit {
		units = append(units, unit)
	}
	sort.Strings(slashSeparatedIds(units))

	for _, unit := range units {
		// Then sort by storage ids
		byStorage := byUnit[unit]
		storageIds := make([]string, 0, len(byStorage))
		for storageId := range byStorage {
			storageIds = append(storageIds, storageId)
		}
		sort.Strings(slashSeparatedIds(storageIds))

		for _, storageId := range storageIds {
			info := byStorage[storageId]
			var sizeStr string
			if size := storageSize[storageId]; size > 0 {
				sizeStr = humanize.IBytes(size * humanize.MiByte)
			}
			w.Print(info.unitId)
			w.Print(info.storageId)
			w.Print(info.kind)
			if len(storagePool) > 0 {
				w.Print(storagePool[info.storageId])
			}
			w.Print(
				storageProviderId[info.storageId],
				sizeStr,
			)
			w.PrintStatus(info.status.Current)
			w.Println(info.status.Message)
		}
	}
	tw.Flush()

	return nil
}

type storageAttachmentInfo struct {
	storageId string
	unitId    string
	kind      string
	status    EntityStatus
}

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
