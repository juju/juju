// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"path"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
func (c *Connection) AvailabilityZones(ctx context.Context, region string) ([]*compute.Zone, error) {
	call := c.Zones.List(c.projectID).
		Context(ctx)
	if region != "" {
		call = call.Filter("name eq " + region + "-.*")
	}

	var results []*compute.Zone
	for {
		zoneList, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, zone := range zoneList.Items {
			results = append(results, zone)
		}
		if zoneList.NextPageToken == "" {
			break
		}
		call = call.PageToken(zoneList.NextPageToken)
	}
	return results, nil
}

// AddInstance creates a new instance based on the spec's data and
// returns it. The instance will be created using the provided
// connection and in the provided zone.
func (c *Connection) AddInstance(ctx context.Context, spec *compute.Instance) (*compute.Instance, error) {
	var waitErr error
	call := c.Service.Instances.Insert(c.projectID, spec.Zone, spec).
		Context(ctx)
	operation, err := call.Do()
	if err != nil {
		// We are guaranteed the insert failed at the point.
		return nil, errors.Annotate(err, "sending new instance request")
	}
	err = c.waitOperation(c.projectID, operation, longRetryStrategy, logOperationErrors)

	if isWaitError(err) {
		waitErr = err
	} else if err != nil {
		// We are guaranteed the insert failed at the point.
		return nil, errors.Annotate(err, "sending new instance request")
	}

	// Check if the instance was created.
	realized, err := c.Instance(ctx, spec.Zone, spec.Name)
	if err != nil {
		if waitErr != nil {
			return nil, errors.Trace(waitErr)
		}
		return nil, errors.Trace(err)
	}
	return realized, nil
}

// Instance gets the up-to-date info about the given instance
// and returns it.
func (c *Connection) Instance(ctx context.Context, zone, id string) (*compute.Instance, error) {
	call := c.Service.Instances.Get(c.projectID, zone, id).
		Context(ctx)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

// Instances sends a request to the GCE API for a list of all instances
// (in the Connection's project) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (c *Connection) Instances(ctx context.Context, prefix string, statuses ...string) ([]*compute.Instance, error) {
	call := c.Service.Instances.AggregatedList(c.projectID).
		Context(ctx)
	call = call.Filter("name eq " + prefix + ".*")

	var results []*compute.Instance
	for {
		rawResult, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, instList := range rawResult.Items {
			for _, inst := range instList.Instances {
				if !checkInstStatus(inst, statuses) {
					continue
				}
				results = append(results, inst)
			}
		}
		if rawResult.NextPageToken == "" {
			break
		}
		call = call.PageToken(rawResult.NextPageToken)
	}
	return results, nil
}

func checkInstStatus(inst *compute.Instance, statuses []string) bool {
	if len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if inst.Status == status {
			return true
		}
	}
	return false
}

// removeInstance sends a request to the GCE API to remove the instance
// with the provided ID (in the specified zone). The call blocks until
// the instance is removed (or the request fails).
func (c *Connection) removeInstance(ctx context.Context, id, zone string) error {
	call := c.Service.Instances.Delete(c.projectID, zone, id).
		Context(ctx)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = c.waitOperation(c.projectID, operation, longRetryStrategy, returnNotFoundOperationErrors)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		// TODO(ericsnow) Try removing the firewall anyway?
		return errors.Trace(err)
	}

	fwname := id
	err = c.RemoveFirewall(ctx, fwname)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// RemoveInstances sends a request to the GCE API to terminate all
// instances (in the Connection's project) that match one of the
// provided IDs. If a prefix is provided, only IDs that start with the
// prefix will be considered. The call blocks until all the instances
// are removed or the request fails.
func (c *Connection) RemoveInstances(ctx context.Context, prefix string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := c.Instances(ctx, prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	// TODO(ericsnow) Remove instances in parallel?
	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.Name == instID {
				zoneName := path.Base(inst.Zone)
				if err := c.removeInstance(ctx, instID, zoneName); err != nil {
					failed = append(failed, instID)
					logger.Errorf("while removing instance %q: %v", instID, err)
				}
				break
			}
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("some instance removals failed: %v", failed)
	}
	return nil
}

// UpdateMetadata sets the metadata key to the specified value for
// all of the instance ids given. The call blocks until all
// of the instances are updated or the request fails.
func (c *Connection) UpdateMetadata(ctx context.Context, key, value string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := c.Instances(ctx, "")
	if err != nil {
		return errors.Annotatef(err, "updating metadata for instances %v", ids)
	}
	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.Name == instID {
				if err := c.updateInstanceMetadata(ctx, inst, key, value); err != nil {
					failed = append(failed, instID)
					logger.Errorf("while updating metadata for instance %q (%v=%q): %v",
						instID, key, value, err)
				}
				break
			}
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("some metadata updates failed: %v", failed)
	}
	return nil

}

// ListMachineTypes returns a list of MachineType available for the
// given zone.
func (c *Connection) ListMachineTypes(ctx context.Context, zone string) ([]*compute.MachineType, error) {
	op := c.MachineTypes.List(c.projectID, zone).
		Context(ctx)
	machines, err := op.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "listing machine types for project %q and zone %q", c.projectID, zone)
	}
	return machines.Items, nil
}

func (c *Connection) updateInstanceMetadata(ctx context.Context, instance *compute.Instance, key, value string) error {
	metadata := instance.Metadata
	existingItem := findMetadataItem(metadata.Items, key)
	if existingItem != nil && existingItem.Value != nil && *existingItem.Value == value {
		// The value's already right.
		return nil
	} else if existingItem == nil {
		metadata.Items = append(metadata.Items, &compute.MetadataItems{Key: key, Value: &value})
	} else {
		existingItem.Value = &value
	}
	// The GCE API won't accept a full URL for the zone (lp:1667172).
	zoneName := path.Base(instance.Zone)
	return errors.Trace(c.setMetadata(ctx, c.projectID, zoneName, instance.Name, metadata))
}

func findMetadataItem(items []*compute.MetadataItems, key string) *compute.MetadataItems {
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.Key == key {
			return item
		}
	}
	return nil
}

func (c *Connection) setMetadata(ctx context.Context, projectID, zone, instanceID string, metadata *compute.Metadata) error {
	call := c.Service.Instances.SetMetadata(projectID, zone, instanceID, metadata).
		Context(ctx)
	op, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = c.waitOperation(projectID, op, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}
