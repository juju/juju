// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatListTabular returns a tabular summary of storage instances.
func formatListTabular(value interface{}) ([]byte, error) {
	storageInfo, ok := value.(map[string]StorageInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", storageInfo, value)
	}
	var out bytes.Buffer
	// To format things into columns.
	tw := tabwriter.NewWriter(&out, 0, 1, 1, ' ', 0)
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%v\t", v)
		}
		fmt.Fprintln(tw)
	}
	p("[Storage]")
	p("UNIT\tID\tLOCATION\tSTATUS\tMESSAGE")

	byUnit := make(map[string]map[string]storageAttachmentInfo)
	for storageId, storageInfo := range storageInfo {
		if storageInfo.Attachments == nil {
			byStorage := byUnit[""]
			if byStorage == nil {
				byStorage = make(map[string]storageAttachmentInfo)
				byUnit[""] = byStorage
			}
			byStorage[storageId] = storageAttachmentInfo{
				storageId:  storageId,
				kind:       storageInfo.Kind,
				persistent: storageInfo.Persistent,
				status:     storageInfo.Status,
			}
			continue
		}
		for unitId, a := range storageInfo.Attachments.Units {
			byStorage := byUnit[unitId]
			if byStorage == nil {
				byStorage = make(map[string]storageAttachmentInfo)
				byUnit[unitId] = byStorage
			}
			byStorage[storageId] = storageAttachmentInfo{
				storageId:  storageId,
				unitId:     unitId,
				kind:       storageInfo.Kind,
				persistent: storageInfo.Persistent,
				location:   a.Location,
				status:     storageInfo.Status,
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
			p(info.unitId, info.storageId, info.location, info.status.Current, info.status.Message)
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}

type storageAttachmentInfo struct {
	storageId  string
	unitId     string
	kind       string
	persistent bool
	location   string
	status     EntityStatus
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
