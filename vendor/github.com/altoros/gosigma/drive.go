// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"
	"time"

	"github.com/altoros/gosigma/data"
)

const (
	// DriveUnmounted defines constant for unmounted drive status
	DriveUnmounted = "unmounted"
	// DriveCreating defines constant for creating drive status
	DriveCreating = "creating"
	// DriveResizing defines constant for resizing drive status
	DriveResizing = "resizing"
	// DriveCloningDst defines constant for drive cloning status
	DriveCloningDst = "cloning_dst"
	// ... may be another values here, contact CloudSigma devs
)

const (
	// MediaCdrom defines media type for cdrom drives
	MediaCdrom = "cdrom"
	// MediaDisk defines media type for disk drives
	MediaDisk = "disk"
)

// A Drive interface represents drive instance in CloudSigma account
type Drive interface {
	// CloudSigma resource
	Resource

	// Affinities
	Affinities() []string

	// AllowMultimount
	AllowMultimount() bool

	// Get meta-information value stored in the drive instance
	Get(key string) (v string, ok bool)

	// Media of drive instance
	Media() string

	// Name of drive instance
	Name() string

	// Owner of drive instance
	Owner() Resource

	// Size of drive in bytes
	Size() uint64

	// Status of drive instance
	Status() string

	// StorageType of drive instance
	StorageType() string

	// IsLibrary returns true if this drive is CloudSigma library drive
	Library() LibrarySpec

	// OS returns operating system of the drive (defined for library drives)
	OS() string

	// Arch returns operating system bit architecture the drive (defined for library drives)
	Arch() string

	// Paid image or free (defined for library drives)
	Paid() bool

	// ImageType returns type of drive image (defined for library drives)
	ImageType() string

	// Clone drive instance
	Clone(params CloneParams, avoid []string) (Drive, error)

	// Clone drive instance, wait for operation finished
	CloneWait(params CloneParams, avoid []string) (Drive, error)

	// Jobs for this drive instance.
	// Every job object in resulting slice carries only UUID and URI.
	// To obtain additional information for job, one should use Job.Refresh() method
	// to query cloud for detailed job information.
	Jobs() []Job

	// Refresh information about drive instance
	Refresh() error

	// Resize drive instance
	Resize(newSize uint64) error

	// Resize drive instance, wait for operation finished
	ResizeWait(newSize uint64) error

	// Wait for user-defined event
	Wait(stop func(Drive) bool) error

	// Remove drive
	Remove() error
}

// A drive implements drive instance in CloudSigma account
type drive struct {
	client  *Client
	obj     *data.Drive
	library LibrarySpec
}

var _ Drive = (*drive)(nil)

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (d drive) String() string {
	return fmt.Sprintf("{Name: %q\nURI: %q\nStatus: %s\nUUID: %q\nSize: %d\nMedia: %s\nStorage: %s}",
		d.Name(), d.URI(), d.Status(), d.UUID(), d.Size(), d.Media(), d.StorageType())
}

// URI of drive instance
func (d drive) URI() string { return d.obj.URI }

// UUID of drive instance
func (d drive) UUID() string { return d.obj.UUID }

// Affinities
func (d drive) Affinities() []string { return d.obj.Affinities }

// AllowMultimount
func (d drive) AllowMultimount() bool { return d.obj.AllowMultimount }

// Get meta-information value stored in the drive instance
func (d drive) Get(key string) (v string, ok bool) {
	v, ok = d.obj.Meta[key]
	return
}

// Media of drive instance
func (d drive) Media() string { return d.obj.Media }

// Name of drive instance
func (d drive) Name() string { return d.obj.Name }

// Owner of drive instance
func (d drive) Owner() Resource {
	if d.obj.Owner == nil {
		return nil
	}
	return &resource{d.obj.Owner}
}

// Size of drive in bytes
func (d drive) Size() uint64 { return d.obj.Size }

// Status of drive instance
func (d drive) Status() string { return d.obj.Status }

// StorageType of drive instance
func (d drive) StorageType() string { return d.obj.StorageType }

// IsLibrary returns true if this drive is CloudSigma library drive
func (d drive) Library() LibrarySpec { return d.library }

// OS returns operating system of the drive (defined for library drives)
func (d drive) OS() string { return d.obj.OS }

// Arch returns operating system bit architecture the drive (defined for library drives)
func (d drive) Arch() string { return d.obj.Arch }

// Paid image or free (defined for library drives)
func (d drive) Paid() bool { return d.obj.Paid }

// ImageType returns type of drive image (defined for library drives)
func (d drive) ImageType() string { return d.obj.ImageType }

// Clone drive instance.
func (d drive) Clone(params CloneParams, avoid []string) (Drive, error) {
	obj, err := d.clone(params, avoid)
	if err != nil {
		return nil, err
	}

	newDrive := &drive{
		client:  d.client,
		obj:     obj,
		library: LibraryAccount,
	}

	return newDrive, nil
}

// Clone drive instance, wait for operation finished.
func (d drive) CloneWait(params CloneParams, avoid []string) (Drive, error) {
	obj, err := d.clone(params, avoid)
	if err != nil {
		return nil, err
	}

	newDrive := &drive{
		client:  d.client,
		obj:     obj,
		library: LibraryAccount,
	}

	jj := newDrive.Jobs()
	if len(jj) == 0 {
		return newDrive, nil
	}

	j := jj[0]

	if err := j.Wait(); err != nil {
		return nil, err
	}

	if err := newDrive.Refresh(); err != nil {
		return nil, err
	}

	if d.Library() == LibraryMedia {
		obj.LibraryDrive = d.obj.LibraryDrive
	}

	return newDrive, nil
}

// Jobs for this drive instance.
// Every job object in resulting slice carries only UUID and URI.
// To obtain additional information for job, one should use Job.Refresh() method
// to query cloud for detailed job information.
func (d drive) Jobs() []Job {
	r := make([]Job, 0, len(d.obj.Jobs))
	for _, j := range d.obj.Jobs {
		j := &job{d.client, &data.Job{Resource: j}}
		r = append(r, j)
	}
	return r
}

// Refresh information about drive instance
func (d *drive) Refresh() error {
	obj, err := d.client.getDrive(d.UUID(), d.Library())
	if err != nil {
		return err
	}
	d.obj = obj
	return nil
}

// Resize drive instance
func (d *drive) Resize(newSize uint64) error {
	return d.resize(newSize)
}

// Resize drive instance, wait for operation finished
func (d *drive) ResizeWait(newSize uint64) error {
	// try to resize drive
	if err := d.resize(newSize); err != nil {
		return err
	}

	// wait for status 'resizing' changes back to 'unmounted'
	return d.Wait(func(d Drive) bool {
		return d.Status() == DriveUnmounted
	})
}

// Wait for user-defined event
func (d *drive) Wait(stop func(Drive) bool) error {
	var timedout = false

	timeout := d.client.GetOperationTimeout()
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { timedout = true })
		defer timer.Stop()
	}

	for !stop(d) {
		if err := d.Refresh(); err != nil {
			return err
		}
		if timedout {
			return ErrOperationTimeout
		}
	}

	return nil
}

// Remove drive
func (d *drive) Remove() error {
	return d.client.RemoveDrive(d.UUID(), d.Library())
}
