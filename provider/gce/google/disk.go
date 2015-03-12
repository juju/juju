// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
)

// The different types of disks supported by GCE.
const (
	diskTypeScratch    = "SCRATCH"
	diskTypePersistent = "PERSISTENT"
)

// The different disk modes supported by GCE.
const (
	diskModeRW = "READ_WRITE"
	diskModeRO = "READ_ONLY"
)

// MinDiskSizeGB is the minimum/default size (in megabytes) for
// GCE disks.
//
// Note: GCE does not currently have an official minimum disk size.
// However, a size of 0 is not viable so we use the next lowest
// value.
const MinDiskSizeGB uint64 = 1

// DiskSpec holds all the data needed to request a new disk on GCE.
// Some fields are used only for attached disks (i.e. in association
// with instances).
type DiskSpec struct {
	// SizeHintGB is the requested disk size in Gigabytes. It must be
	// greater than 0.
	SizeHintGB uint64
	// ImageURL is the location of the image to which the disk should
	// be initialized.
	ImageURL string
	// Boot indicates that this is a boot disk. An instance may only
	// have one boot disk. (attached only)
	Boot bool
	// Scratch indicates that the disk should be a "scratch" disk
	// instead of a "persistent" disk (the default).
	Scratch bool
	// Readonly indicates that the disk should not support writes.
	Readonly bool
	// AutoDelete indicates that the attached disk should be removed
	// when the instance to which it is attached is removed.
	AutoDelete bool
}

// TooSmall checks the spec's size hint and indicates whether or not
// it is smaller than the minimum disk size.
func (ds *DiskSpec) TooSmall() bool {
	return ds.SizeHintGB < MinDiskSizeGB
}

// SizeGB returns the disk size to use for a new disk. The size hint
// is returned if it isn't too small (otherwise the min size is
// returned).
func (ds *DiskSpec) SizeGB() uint64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB
	}
	return size
}

// newAttached builds a compute.AttachedDisk using the information in
// the disk spec and returns it.
//
// Note: Not all AttachedDisk fields are set.
func (ds *DiskSpec) newAttached() *compute.AttachedDisk {
	// TODO(ericsnow) Fail if SizeHintGB is 0?
	diskType := diskTypePersistent
	if ds.Scratch {
		diskType = diskTypeScratch
	}
	mode := diskModeRW
	if ds.Readonly {
		mode = diskModeRO
	}

	disk := compute.AttachedDisk{
		Type:       diskType,
		Boot:       ds.Boot,
		Mode:       mode,
		AutoDelete: ds.AutoDelete,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: int64(ds.SizeGB()),
			// DiskType (defaults to pd-standard, pd-ssd, local-ssd)
			SourceImage: ds.ImageURL,
		},
		// Interface (defaults to SCSI)
		// DeviceName (GCE sets this, persistent disk only)
	}
	return &disk
}
