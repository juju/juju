// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
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

const (
	// MinDiskSizeGB is the minimum/default size (in megabytes) for
	// GCE disks.
	//
	// Note: GCE does not currently have a minimum disk size.
	MinDiskSizeGB int64 = 0
)

// DiskSpec holds all the data needed to request a new disk on GCE.
// Some fields are used only for attached disks (i.e. in association
// with instances).
type DiskSpec struct {
	// sizeHintGB is the requested disk size in Gigabytes.
	SizeHintGB int64
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
// TODO(ericsnow) Return uint64?
func (ds *DiskSpec) SizeGB() int64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB
	}
	return size
}

// newAttached builds a compute.AttachedDisk using the information in
// the disk spec and returns it.
//
// Npte: Not all AttachedDisk fields are set.
func (ds *DiskSpec) newAttached() *compute.AttachedDisk {
	diskType := diskTypePersistent // The default.
	if ds.Scratch {
		diskType = diskTypeScratch
	}
	mode := diskModeRW // The default.
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
			DiskSizeGb: ds.SizeGB(),
			// DiskType (defaults to pd-standard, pd-ssd, local-ssd)
			SourceImage: ds.ImageURL,
		},
		// Interface (defaults to SCSI)
		// DeviceName (GCE sets this, persistent disk only)
	}
	return &disk
}

// rootDisk identifies the root disk for a given instance (or instance
// spec) and returns it. If the root disk could not be determined then
// nil is returned.
// TODO(ericsnow) Return an error?
func rootDisk(inst interface{}) *compute.AttachedDisk {
	switch typed := inst.(type) {
	case *compute.Instance:
		return typed.Disks[0]
	case *Instance:
		if typed.spec == nil {
			return nil
		}
		return typed.spec.Disks[0].newAttached()
	case *InstanceSpec:
		return typed.Disks[0].newAttached()
	default:
		panic(inst)
		return nil
	}
}

// diskSizeGB determines the size of the provided disk. This works both
// with compute.Disk and compute.AttachedDisk. For attached disks the
// size is pulled from the data used initialize it. If that data is not
// available then an error is returned.
func diskSizeGB(disk interface{}) (int64, error) {
	switch typed := disk.(type) {
	case *compute.Disk:
		return typed.SizeGb, nil
	case *compute.AttachedDisk:
		if typed.InitializeParams == nil {
			return 0, errors.Errorf("attached disk missing init params: %v", disk)
		}
		return typed.InitializeParams.DiskSizeGb, nil
	default:
		return 0, errors.Errorf("disk has unrecognized type: %v", disk)
	}
}
