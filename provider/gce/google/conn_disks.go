// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

// CreateDisks implements storage section of gceConnection.
func (gce *Connection) CreateDisks(zone string, disks []DiskSpec) ([]*Disk, error) {
	results := make([]*Disk, len(disks))
	for i, disk := range disks {
		d, err := disk.newDetached()
		if err != nil {
			return []*Disk{}, errors.Annotate(err, "cannot create disk spec")
		}
		if err := gce.createDisk(zone, d); err != nil {
			return []*Disk{}, errors.Annotatef(err, "cannot create disk %q", disk.Name)
		}
		results[i] = NewDisk(d)
	}
	return results, nil
}

func (gce *Connection) createDisk(zone string, disk *compute.Disk) error {
	return gce.raw.CreateDisk(gce.projectID, zone, disk)
}

// Disks implements storage section of gceConnection.
func (gce *Connection) Disks(zone string) ([]*Disk, error) {
	computeDisks, err := gce.raw.ListDisks(gce.projectID, zone)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot list disks for zone %q", zone)
	}
	disks := make([]*Disk, len(computeDisks))
	for i, disk := range computeDisks {
		disks[i] = NewDisk(disk)
	}
	return disks, nil
}

// RemoveDisk implements storage section of gceConnection.
// TODO(perrito666) handle non existing disk, perhaps catch 404.
func (gce *Connection) RemoveDisk(zone, name string) error {
	return gce.raw.RemoveDisk(gce.projectID, zone, name)
}

// Disk implements storage section of gceConnection.
func (gce *Connection) Disk(zone, name string) (*Disk, error) {
	d, err := gce.raw.GetDisk(gce.projectID, zone, name)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get disk %q in zone %q", name, zone)
	}
	return NewDisk(d), nil
}

// deviceName will generate a device name from the passed
// <zone> and <diskId>, the device name must not be confused
// with the volume name, as it is used mainly to name the
// disk when attached to a linux OS.
func deviceName(zone string, diskId uint64) string {
	return fmt.Sprintf("%s-%d", zone, diskId)
}

// AttachDisk implements storage section of gceConnection.
func (gce *Connection) AttachDisk(zone, volumeName, instanceId string, mode DiskMode) (*AttachedDisk, error) {
	disk, err := gce.raw.GetDisk(gce.projectID, zone, volumeName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot obtain disk %q to attach it", volumeName)
	}
	attachedDisk := &compute.AttachedDisk{
		// Specifies a unique device name of your choice that
		// is reflected into the /dev/disk/by-id/google-*
		DeviceName: deviceName(zone, disk.Id),
		Source:     disk.SelfLink,
		Mode:       string(mode),
	}
	err = gce.raw.AttachDisk(gce.projectID, zone, instanceId, attachedDisk)
	if err != nil {
		return nil, errors.Annotate(err, "cannot attach disk")
	}
	return &AttachedDisk{
		VolumeName: volumeName,
		DeviceName: attachedDisk.DeviceName,
		Mode:       mode,
	}, nil
}

// DetachDisk implements storage section of gceConnection.
// disk existence is checked but not instance nor is attachment.
func (gce *Connection) DetachDisk(zone, instanceId, volumeName string) error {
	disk, err := gce.raw.GetDisk(gce.projectID, zone, volumeName)
	if err != nil {
		return errors.Annotatef(err, "cannot obtain disk %q to detach it", volumeName)
	}
	dn := deviceName(zone, disk.Id)
	err = gce.raw.DetachDisk(gce.projectID, zone, instanceId, dn)
	if err != nil {
		return errors.Annotatef(err, "cannot detach %q from %q", dn, instanceId)
	}
	return nil
}

// sourceToVolumeName will return the disk Name part of a
// source URL for a compute disk, compute is a bit inconsistent
// on its handling of disk resources, when used in requests it will
// take the disk.Name but when used as a parameter it will take
// the source url.
// The source url (short) format is:
// /projects/project/zones/zone/disks/disk
// the relevant part is disk.
func sourceToVolumeName(source string) string {
	if source == "" {
		return ""
	}
	parts := strings.Split(source, "/")
	if len(parts) == 1 {
		return source
	}
	lastItem := len(parts) - 1
	return parts[lastItem]
}

// InstanceDisks implements storage section of gceConnection.
func (gce *Connection) InstanceDisks(zone, instanceId string) ([]*AttachedDisk, error) {
	disks, err := gce.raw.InstanceDisks(gce.projectID, zone, instanceId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get disks from instance")
	}
	att := make([]*AttachedDisk, len(disks))
	for i, disk := range disks {
		att[i] = &AttachedDisk{
			VolumeName: sourceToVolumeName(disk.Source),
			DeviceName: disk.DeviceName,
			Mode:       DiskMode(disk.Mode),
		}
	}
	return att, nil
}
