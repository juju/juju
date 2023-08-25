// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "sort"

// SortBlockDevices sorts block devices by device name.
func SortBlockDevices(devices []BlockDevice) {
	sort.Sort(byDeviceName(devices))
}

type byDeviceName []BlockDevice

func (b byDeviceName) Len() int {
	return len(b)
}

func (b byDeviceName) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byDeviceName) Less(i, j int) bool {
	return b[i].DeviceName < b[j].DeviceName
}
