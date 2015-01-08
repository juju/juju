// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
)

const (
	diskTypeScratch    = "SCRATCH"
	diskTypePersistent = "PERSISTENT"
	diskModeRW         = "READ_WRITE"
	diskModeRO         = "READ_ONLY"

	// MinDiskSize is the minimum/default size (in megabytes) for GCE
	// disks. GCE does not currently have a minimum disk size.
	MinDiskSizeGB int64 = 0
)

type DiskSpec struct {
	// sizeHint is the requested disk size in Gigabytes.
	SizeHintGB int64
	ImageURL   string
	Boot       bool
	Scratch    bool
	Readonly   bool
	AutoDelete bool
}

func (ds *DiskSpec) TooSmall() bool {
	return ds.SizeHintGB < MinDiskSizeGB
}

func (ds *DiskSpec) SizeGB() int64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB
	}
	return size
}

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
		return nil
	}
}

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
