// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
)

var (
	RootDisk   = rootDisk
	DiskSizeGB = diskSizeGB
)

func NewAttached(spec DiskSpec) *compute.AttachedDisk {
	return spec.newAttached()
}
