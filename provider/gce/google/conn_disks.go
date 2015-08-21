// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

func (gce *Connection) CreateDisks(zone string, disks []DiskSpec) ([]*compute.Disk, error) {
	results := make([]*compute.Disk, len(disks))
	for i, disk := range disks {
		d, err := disk.newDetached()
		if err != nil {
			return []*compute.Disk{}, errors.Annotate(err, "cannot create disk spec")
		}
		if err := gce.createDisk(zone, d); err != nil {
			return []*compute.Disk{}, errors.Annotatef(err, "cannot create disk %q", disk.Name)
		}
		results[i] = d
	}
	return results, nil
}

func (gce *Connection) createDisk(zone string, disk *compute.Disk) error {
	return gce.raw.CreateDisk(gce.projectID, zone, disk)
}

func (gce *Connection) Disks(zone string) ([]*compute.Disk, error) {
	return gce.raw.ListDisks(gce.projectID, zone)
}

func (gce *Connection) RemoveDisk(zone, id string) error {
	return gce.raw.RemoveDisk(gce.projectID, zone, id)
}

func (gce *Connection) Disk(zone, id string) (*compute.Disk, error) {
	return gce.raw.GetDisk(gce.projectID, zone, id)
}

func deviceName(zone string, diskId uint64) string {
	return fmt.Sprintf("%s-%d", zone, diskId)
}

func (gce *Connection) AttachDisk(zone, volumeName, instanceId, mode string) (*AttachedDisk, error) {
	disk, err := gce.raw.GetDisk(gce.projectID, zone, volumeName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot obtain disk %q to attach it", volumeName)
	}
	attachedDisk := &compute.AttachedDisk{
		// Specifies a unique device name of your choice that
		// is reflected into the /dev/disk/by-id/google-*
		DeviceName: deviceName(zone, disk.Id),
		Source:     disk.SelfLink,
		Mode:       mode,
	}
	err = gce.raw.AttachDisk(gce.projectID, zone, instanceId, attachedDisk)
	if err != nil {
		return nil, errors.Annotate(err, "cannot attach disk")
	}
	return &AttachedDisk{
		VolumeName: volumeName,
		DeviceName: attachedDisk.DeviceName,
		Mode:       attachedDisk.Mode,
	}, nil
}

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
			Mode:       disk.Mode,
		}
	}
	return att, nil
}
