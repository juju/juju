// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

// The different types of disk persistence supported by GCE.
const (
	diskPersistenceTypeScratch    = "SCRATCH"
	diskPersistenceTypePersistent = "PERSISTENT"
)

// The types of disk supported by GCE
const (
	// persistent
	DiskTypePersistentStandard = "pd-standard"
	DiskTypePersistentSSD      = "pd-ssd"
	// scratch
	DiskTypeLocalSSD = "local-ssd"
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
// However, in testing we found the minimum size to be 10 GB due to
// the image size. See gceapi messsage.
//
// gceapi: Requested disk size cannot be smaller than the image size (10 GB)
const MinDiskSizeGB uint64 = 10

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
	// PersistenDiskType is exclusive to ssd and indicates which of the persistent
	// types available this disk should be.
	PersistentDiskType string
	// Name: Name of the resource; provided by the client when the resource
	// is created. The name must be 1-63 characters long, and comply with
	// RFC1035. Specifically, the name must be 1-63 characters long and
	// match the regular expression [a-z]([-a-z0-9]*[a-z0-9])? which means
	// the first character must be a lowercase letter, and all following
	// characters must be a dash, lowercase letter, or digit, except the
	// last character, which cannot be a dash.
	Name string
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
	diskType := diskPersistenceTypePersistent
	if ds.Scratch {
		diskType = diskPersistenceTypeScratch
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

// newDetached creates a new detached persistent disk representation,
// this DOES NOT create a disk in gce, just creates the spec.
// reference in https://cloud.google.com/compute/docs/reference/latest/disks#resource
func (ds *DiskSpec) newDetached() (*compute.Disk, error) {
	if ds.Scratch {
		return nil, errors.New("cannot create scratch volumes detached")
	}
	if ds.PersistentDiskType == DiskTypeLocalSSD {
		return nil, errors.New("cannot create local ssd disks detached")
	}
	return &compute.Disk{
		Name:        ds.Name,
		SizeGb:      int64(ds.SizeGB()),
		SourceImage: ds.ImageURL,
		Type:        ds.PersistentDiskType,
	}, nil
}

type AttachedDisk struct {
	VolumeName string
	DeviceName string
	Mode       string
}
