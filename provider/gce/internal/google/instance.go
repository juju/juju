// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"path"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"google.golang.org/api/iterator"
)

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
func (c *Connection) AvailabilityZones(ctx context.Context, region string) ([]*computepb.Zone, error) {
	req := &computepb.ListZonesRequest{
		Project: c.projectID,
	}
	if region != "" {
		filter := "name eq " + region + "-.*"
		req.Filter = &filter
	}
	iter := c.zones.List(ctx, req)
	return fetchResults[computepb.Zone](iter.Next, "availability zones")
}

// AddInstance creates a new instance based on the spec's data and
// returns it. The instance will be created using the provided
// connection and in the provided zone.
func (c *Connection) AddInstance(ctx context.Context, spec *computepb.Instance) (*computepb.Instance, error) {
	op, err := c.instances.Insert(ctx, &computepb.InsertInstanceRequest{
		Project:          c.projectID,
		Zone:             spec.GetZone(),
		InstanceResource: spec,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "adding new instance %q", spec.GetName())
	}

	// Check if the instance was created.
	realized, err := c.Instance(ctx, spec.GetZone(), spec.GetName())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return realized, nil
}

// Instance gets the up-to-date info about the given instance
// and returns it.
func (c *Connection) Instance(ctx context.Context, zone, id string) (*computepb.Instance, error) {
	inst, err := c.instances.Get(ctx, &computepb.GetInstanceRequest{
		Project:  c.projectID,
		Zone:     zone,
		Instance: id,
	})
	return inst, errors.Trace(err)
}

// Instances sends a request to the GCE API for a list of all instances
// (in the Connection's project) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (c *Connection) Instances(ctx context.Context, prefix string, statuses ...string) ([]*computepb.Instance, error) {
	filter := "name eq " + prefix + ".*"
	iter := c.instances.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project:              c.projectID,
		Filter:               &filter,
		ReturnPartialSuccess: &trueVal,
	})
	var results []*computepb.Instance
	for {
		instList, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, inst := range instList.Value.GetInstances() {
			if !checkInstStatus(inst, statuses) {
				continue
			}
			results = append(results, inst)
		}
	}
	return results, nil
}

func checkInstStatus(inst *computepb.Instance, statuses []string) bool {
	if len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if inst.GetStatus() == status {
			return true
		}
	}
	return false
}

// removeInstance sends a request to the GCE API to remove the instance
// with the provided ID (in the specified zone). The call blocks until
// the instance is removed (or the request fails).
func (c *Connection) removeInstance(ctx context.Context, id, zone string) error {
	op, err := c.instances.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project:  c.projectID,
		Zone:     zone,
		Instance: id,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
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
			if inst.GetName() == instID {
				zoneName := path.Base(inst.GetZone())
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
			if inst.GetName() == instID {
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
func (c *Connection) ListMachineTypes(ctx context.Context, zone string) ([]*computepb.MachineType, error) {
	iter := c.machineTypes.List(ctx, &computepb.ListMachineTypesRequest{
		Project: c.projectID,
		Zone:    zone,
	})
	return fetchResults[computepb.MachineType](iter.Next, "machine types")
}

func (c *Connection) updateInstanceMetadata(ctx context.Context, instance *computepb.Instance, key, value string) error {
	metadata := instance.GetMetadata()
	existingItem := findMetadataItem(metadata.GetItems(), key)
	if existingItem != nil && existingItem.Value != nil && *existingItem.Value == value {
		// The value's already right.
		return nil
	} else if existingItem == nil {
		metadata.Items = append(metadata.Items, &computepb.Items{Key: &key, Value: &value})
	} else {
		existingItem.Value = &value
	}
	// The GCE API won't accept a full URL for the zone (lp:1667172).
	zoneName := path.Base(instance.GetZone())
	return errors.Trace(c.setMetadata(ctx, zoneName, instance.GetName(), metadata))
}

func findMetadataItem(items []*computepb.Items, key string) *computepb.Items {
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.GetKey() == key {
			return item
		}
	}
	return nil
}

func (c *Connection) setMetadata(ctx context.Context, zone, instanceID string, metadata *computepb.Metadata) error {
	op, err := c.instances.SetMetadata(ctx, &computepb.SetMetadataInstanceRequest{
		Instance:         instanceID,
		Project:          c.projectID,
		Zone:             zone,
		MetadataResource: metadata,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "updating instance %q metadata", instanceID)
}
