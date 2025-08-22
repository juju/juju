// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
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

func formatDiskType(project, zone string, spec *compute.Disk) {
	// empty will default in pd-standard
	if spec.Type == "" {
		return
	}
	// see https://cloud.google.com/compute/docs/reference/latest/disks#resource
	if strings.HasPrefix(spec.Type, "http") || strings.HasPrefix(spec.Type, "projects") || strings.HasPrefix(spec.Type, "global") {
		return
	}
	spec.Type = fmt.Sprintf(diskTypesBase, project, zone, spec.Type)
}

func (c *Connection) CreateDisk(ctx context.Context, zone string, spec *compute.Disk) error {
	formatDiskType(c.projectID, zone, spec)
	call := c.Service.Disks.Insert(c.projectID, zone, spec).
		Context(ctx)
	op, err := call.Do()
	if err != nil {
		return errors.Annotate(err, "could not create a new disk")
	}
	return errors.Trace(c.waitOperation(c.projectID, op, longRetryStrategy, logOperationErrors))
}

func (c *Connection) Disks(ctx context.Context) ([]*compute.Disk, error) {
	call := c.Service.Disks.AggregatedList(c.projectID).
		Context(ctx)
	var results []*compute.Disk
	for {
		diskList, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, list := range diskList.Items {
			results = append(results, list.Disks...)
		}
		if diskList.NextPageToken == "" {
			break
		}
		call = call.PageToken(diskList.NextPageToken)
	}
	return results, nil
}

func (c *Connection) RemoveDisk(ctx context.Context, zone, id string) error {
	call := c.Service.Disks.Delete(c.projectID, zone, id).
		Context(ctx)
	op, err := call.Do()
	if err != nil {
		return errors.Annotatef(err, "could not delete disk %q", id)
	}
	return errors.Trace(c.waitOperation(c.projectID, op, longRetryStrategy, returnNotFoundOperationErrors))
}

func (c *Connection) Disk(ctx context.Context, zone, id string) (*compute.Disk, error) {
	call := c.Service.Disks.Get(c.projectID, zone, id).
		Context(ctx)
	disk, err := call.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get disk %q at zone %q in project %q", id, zone, c.projectID)
	}
	return disk, nil
}

func (c *Connection) SetDiskLabels(ctx context.Context, zone, id, labelFingerprint string, labels map[string]string) error {
	call := c.Service.Disks.SetLabels(c.projectID, zone, id, &compute.ZoneSetLabelsRequest{
		LabelFingerprint: labelFingerprint,
		Labels:           labels,
	}).Context(ctx)
	_, err := call.Do()
	return errors.Trace(err)
}

// deviceName will generate a device name from the passed
// <zone> and <diskId>, the device name must not be confused
// with the volume name, as it is used mainly to name the
// disk when attached to a linux OS.
func deviceName(zone string, diskId uint64) string {
	return fmt.Sprintf("%s-%d", zone, diskId)
}

// AttachDisk implements storage section of gceConnection.
func (c *Connection) AttachDisk(ctx context.Context, zone, volumeName, instanceId string, mode DiskMode) (*compute.AttachedDisk, error) {
	disk, err := c.Disk(ctx, zone, volumeName)
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
	call := c.Service.Instances.AttachDisk(c.projectID, zone, instanceId, attachedDisk).
		Context(ctx)
	_, err = call.Do() // Perhaps return something from the Op
	if err != nil {
		return nil, errors.Annotatef(err, "cannot attach %q into %q", attachedDisk.DeviceName, instanceId)
	}
	return attachedDisk, nil
}

// DetachDisk implements storage section of gceConnection.
// disk existence is checked but not instance nor is attachment.
func (c *Connection) DetachDisk(ctx context.Context, zone, instanceId, volumeName string) error {
	disk, err := c.Disk(ctx, zone, volumeName)
	if err != nil {
		return errors.Annotatef(err, "cannot obtain disk %q to detach it", volumeName)
	}
	dn := deviceName(zone, disk.Id)
	call := c.Service.Instances.DetachDisk(c.projectID, zone, instanceId, dn)
	_, err = call.Do() // Perhaps return something from the Op
	if err != nil {
		return errors.Annotatef(err, "cannot detach %q from %q", dn, instanceId)
	}
	return nil
}

func (c *Connection) InstanceDisks(ctx context.Context, zone, instanceId string) ([]*compute.AttachedDisk, error) {
	instance, err := c.Instance(ctx, zone, instanceId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get instance %q to list its disks", instanceId)
	}
	return instance.Disks, nil
}

// CreateDisks implements storage section of gceConnection.
func (c *Connection) CreateDisks(ctx context.Context, zone string, disks []*compute.Disk) error {
	for _, d := range disks {
		if err := c.CreateDisk(ctx, zone, d); err != nil {
			return errors.Annotatef(err, "cannot create disk %q", d.Name)
		}
	}
	return nil
}
