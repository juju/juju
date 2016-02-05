// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/errors"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"google.golang.org/api/compute/v1"
)

// The different types of disk persistence supported by GCE.
const (
	diskPersistenceTypeScratch    = "SCRATCH"
	diskPersistenceTypePersistent = "PERSISTENT"
)

type DiskType string

// The types of disk supported by GCE
const (
	// persistent
	DiskPersistentStandard DiskType = "pd-standard"
	DiskPersistentSSD      DiskType = "pd-ssd"
	// scratch
	DiskLocalSSD DiskType = "local-ssd"
)

type DiskMode string

// The different disk modes supported by GCE.
const (
	ModeRW DiskMode = "READ_WRITE"
	ModeRO DiskMode = "READ_ONLY"
)

type DiskStatus string

const (
	StatusCreating  DiskStatus = "CREATING"
	StatusFailed    DiskStatus = "FAILED"
	StatusReady     DiskStatus = "READY"
	StatusRestoring DiskStatus = "RESTORING"
)

// MinDiskSizeGB is the minimum/default size (in megabytes) for
// GCE disks.
//
// Note: GCE does not currently have an official minimum disk size.
// However, in testing we found the minimum size to be 10 GB for ubuntu
// and 50 GB for windows due to the image size. See gceapi message.
//
// gceapi: Requested disk size cannot be smaller than the image size (10 GB)
func MinDiskSizeGB(ser string) uint64 {
	// See comment below that explains why we're ignoring the error
	os, _ := series.GetOSFromSeries(ser)
	switch os {
	case jujuos.Ubuntu:
		return 10
	case jujuos.Windows:
		return 50
	// On default we just return a "sane" default since the error
	// will be propagated through the api and appear in juju status anyway
	default:
		return 10
	}
}

// gibToMib converts gibibytes to mebibytes.
func gibToMib(g int64) uint64 {
	return uint64(g) * 1024
}

// DiskSpec holds all the data needed to request a new disk on GCE.
// Some fields are used only for attached disks (i.e. in association
// with instances).
type DiskSpec struct {
	// Series is the OS series on which the disk size depends
	Series string
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
	// PersistenDiskType is exclusive to persistent disks and indicates which of the
	// persistent types available this disk should be.
	PersistentDiskType DiskType
	// Name: Name of the resource; provided by the client when the resource
	// is created. The name must be 1-63 characters long, and comply with
	// RFC1035. Specifically, the name must be 1-63 characters long and
	// match the regular expression [a-z]([-a-z0-9]*[a-z0-9])? which means
	// the first character must be a lowercase letter, and all following
	// characters must be a dash, lowercase letter, or digit, except the
	// last character, which cannot be a dash.
	Name string
	// Description holds a description of the disk, it currently holds
	// modelUUID.
	// This field is used instead of a tag or metadata because, at the moment of writing
	// this feature, compute (v1) API does not support any way to add extra data
	// to disks.
	// Description was picked because it is not mutable (actually no field is) for disks.
	// There is a metadata API but it is not supported for disks for the moment.
	Description string
}

// TooSmall checks the spec's size hint and indicates whether or not
// it is smaller than the minimum disk size.
func (ds *DiskSpec) TooSmall() bool {
	return ds.SizeHintGB < MinDiskSizeGB(ds.Series)
}

// SizeGB returns the disk size to use for a new disk. The size hint
// is returned if it isn't too small (otherwise the min size is
// returned).
func (ds *DiskSpec) SizeGB() uint64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB(ds.Series)
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
	mode := ModeRW
	if ds.Readonly {
		mode = ModeRO
	}

	disk := compute.AttachedDisk{
		Type:       diskType,
		Boot:       ds.Boot,
		Mode:       string(mode),
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
	if ds.PersistentDiskType == DiskLocalSSD {
		return nil, errors.New("cannot create local ssd disks detached")
	}
	return &compute.Disk{
		Name:        ds.Name,
		SizeGb:      int64(ds.SizeGB()),
		SourceImage: ds.ImageURL,
		Type:        string(ds.PersistentDiskType),
		Description: ds.Description,
	}, nil
}

// AttachedDisk represents a disk that is attached to an instance.
type AttachedDisk struct {
	// VolumeName is the name of the volume that is attached, this is unique
	// and used by gce as an identifier.
	VolumeName string
	// DeviceName is the name of the device in the instance, typycally
	// is reflected into the /dev/disk/by-id/google-*
	DeviceName string
	// Mode is the read/write mode of the disk.
	Mode DiskMode
}

// Disk represents a gce disk.
type Disk struct {
	// Id is an unique identifier google adds to the disk, it usually
	// is not used in the API.
	Id uint64
	// Name is a unique identifier string for each disk.
	Name string
	// Description holds the description field for a disk, we store env UUID here.
	Description string
	// Size is the size in mbit.
	Size uint64
	// Type is one of the available disk types supported by
	// gce (persistent or ephemeral).
	Type DiskType
	// Zone indicates the zone in which the disk lives.
	Zone string
	// DiskStatus holds the status of he aforementioned disk.
	Status DiskStatus
}

func NewDisk(cd *compute.Disk) *Disk {
	d := &Disk{
		Id:          cd.Id,
		Name:        cd.Name,
		Description: cd.Description,
		Size:        gibToMib(cd.SizeGb),
		Type:        DiskType(cd.Type),
		Zone:        cd.Zone,
		Status:      DiskStatus(cd.Status),
	}
	return d
}
