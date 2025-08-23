// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"google.golang.org/api/iterator"
)

// The different types of disk persistence supported by GCE.
const (
	DiskPersistenceTypeScratch    = "SCRATCH"
	DiskPersistenceTypePersistent = "PERSISTENT"
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

const MinDiskSizeGB = 10

const diskTypesBase = "https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s"

func formatDiskType(project, zone string, spec *computepb.Disk) {
	// empty will default in pd-standard
	if spec.GetType() == "" {
		return
	}
	// see https://cloud.google.com/compute/docs/reference/latest/disks#resource
	if strings.HasPrefix(spec.GetType(), "http") || strings.HasPrefix(spec.GetType(), "projects") ||
		strings.HasPrefix(spec.GetType(), "global") {
		return
	}
	specType := fmt.Sprintf(diskTypesBase, project, zone, spec.GetType())
	spec.Type = &specType
}

var trueVal = true

// Disks will return a list of all Disks found in the project.
func (c *Connection) Disks(ctx context.Context) ([]*computepb.Disk, error) {
	iter := c.disks.AggregatedList(ctx, &computepb.AggregatedListDisksRequest{
		Project:              c.projectID,
		ReturnPartialSuccess: &trueVal,
	})
	var results []*computepb.Disk
	for {
		diskList, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		results = append(results, diskList.Value.GetDisks()...)
	}
	return results, nil
}

// RemoveDisk will destroy the disk identified by <name> in <zone>.
func (c *Connection) RemoveDisk(ctx context.Context, zone, id string) error {
	op, err := c.disks.Delete(ctx, &computepb.DeleteDiskRequest{
		Project: c.projectID,
		Zone:    zone,
		Disk:    id,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "deleting disk %q", id)
}

// Disk will return a Disk representing the disk identified by the
// passed <name> or error.
func (c *Connection) Disk(ctx context.Context, zone, id string) (*computepb.Disk, error) {
	disk, err := c.disks.Get(ctx, &computepb.GetDiskRequest{
		Project: c.projectID,
		Zone:    zone,
		Disk:    id,
	})
	return disk, errors.Annotatef(err, "getting disk %q", id)
}

// SetDiskLabels sets the labels on a disk, ensuring that the disk's
// label fingerprint matches the one supplied.
func (c *Connection) SetDiskLabels(ctx context.Context, zone, id, labelFingerprint string, labels map[string]string) error {
	op, err := c.disks.SetLabels(ctx, &computepb.SetLabelsDiskRequest{
		Project: c.projectID,
		Zone:    zone,
		ZoneSetLabelsRequestResource: &computepb.ZoneSetLabelsRequest{
			LabelFingerprint: &labelFingerprint,
			Labels:           labels,
		},
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "setting labels for disk %q", id)
}

// deviceName will generate a device name from the passed
// <zone> and <diskId>, the device name must not be confused
// with the volume name, as it is used mainly to name the
// disk when attached to a linux OS.
func deviceName(zone string, diskId uint64) string {
	return fmt.Sprintf("%s-%d", zone, diskId)
}

// AttachDisk implements storage section of gceConnection.
func (c *Connection) AttachDisk(ctx context.Context, zone, volumeName, instanceId string, mode DiskMode) (*computepb.AttachedDisk, error) {
	disk, err := c.Disk(ctx, zone, volumeName)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot obtain disk %q to attach it", volumeName)
	}
	deviceNameStr := deviceName(zone, disk.GetId())
	modeStr := string(mode)
	attachedDisk := &computepb.AttachedDisk{
		// Specifies a unique device name of your choice that
		// is reflected into the /dev/disk/by-id/google-*
		DeviceName: &deviceNameStr,
		Source:     disk.SelfLink,
		Mode:       &modeStr,
	}
	op, err := c.instances.AttachDisk(ctx, &computepb.AttachDiskInstanceRequest{
		Project:              c.projectID,
		Zone:                 zone,
		Instance:             instanceId,
		AttachedDiskResource: attachedDisk,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return attachedDisk, errors.Annotatef(err, "attaching disk to instance %q", instanceId)
}

// DetachDisk implements storage section of gceConnection.
// disk existence is checked but not instance nor is attachment.
func (c *Connection) DetachDisk(ctx context.Context, zone, instanceId, volumeName string) error {
	disk, err := c.Disk(ctx, zone, volumeName)
	if err != nil {
		return errors.Annotatef(err, "cannot obtain disk %q to detach it", volumeName)
	}
	dn := deviceName(zone, disk.GetId())
	op, err := c.instances.DetachDisk(ctx, &computepb.DetachDiskInstanceRequest{
		Project:    c.projectID,
		Zone:       zone,
		Instance:   instanceId,
		DeviceName: dn,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "detaching %q from %q", dn, instanceId)
}

// InstanceDisks returns a list of the disks attached to the passed instance.
func (c *Connection) InstanceDisks(ctx context.Context, zone, instanceId string) ([]*computepb.AttachedDisk, error) {
	instance, err := c.Instance(ctx, zone, instanceId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get instance %q to list its disks", instanceId)
	}
	return instance.Disks, nil
}

// CreateDisks implements storage section of gceConnection.
func (c *Connection) CreateDisks(ctx context.Context, zone string, disks []*computepb.Disk) error {
	for _, d := range disks {
		if err := c.createDisk(ctx, zone, d); err != nil {
			return errors.Annotatef(err, "cannot create disk %q", d.GetName())
		}
	}
	return nil
}

func (c *Connection) createDisk(ctx context.Context, zone string, spec *computepb.Disk) error {
	formatDiskType(c.projectID, zone, spec)
	op, err := c.disks.Insert(ctx, &computepb.InsertDiskRequest{
		Project:      c.projectID,
		Zone:         zone,
		DiskResource: spec,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotate(err, "creating a new disk")
}
