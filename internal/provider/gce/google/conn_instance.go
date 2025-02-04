// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"path"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

// addInstance sends a request to GCE to add a new instance to the
// connection's project, with the provided instance data and machine
// type. The instance that was passed in is updated with the new
// instance's data upon success. The call blocks until the instance
// is created or the request fails.
// TODO(ericsnow) Return a new inst.
func (gce *Connection) addInstance(requestedInst *compute.Instance, machineType string, zone string) error {
	var waitErr error
	inst := *requestedInst
	inst.MachineType = formatMachineType(zone, machineType)
	err := gce.service.AddInstance(gce.projectID, zone, &inst)
	if isWaitError(err) {
		waitErr = err
	} else if err != nil {
		// We are guaranteed the insert failed at the point.
		return errors.Annotate(err, "sending new instance request")
	}

	// Check if the instance was created.
	realized, err := gce.service.GetInstance(gce.projectID, zone, inst.Name)
	if err != nil {
		if waitErr != nil {
			return errors.Trace(waitErr)
		}
		return errors.Trace(err)
	}

	// Success!
	*requestedInst = *realized
	return nil
}

// AddInstance creates a new instance based on the spec's data and
// returns it. The instance will be created using the provided
// connection and in the provided zone.
func (gce *Connection) AddInstance(spec InstanceSpec) (*Instance, error) {
	raw := spec.raw()
	if err := gce.addInstance(raw, spec.Type, spec.AvailabilityZone); err != nil {
		return nil, errors.Trace(err)
	}

	return newInstance(raw, &spec), nil
}

// Instance gets the up-to-date info about the given instance
// and returns it.
func (gce *Connection) Instance(id, zone string) (Instance, error) {
	var result Instance
	raw, err := gce.service.GetInstance(gce.projectID, zone, id)
	if err != nil {
		return result, errors.Trace(err)
	}
	result = *newInstance(raw, nil)
	return result, nil
}

// Instances sends a request to the GCE API for a list of all instances
// (in the Connection's project) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (gce *Connection) Instances(prefix string, statuses ...string) ([]Instance, error) {
	rawInsts, err := gce.service.ListInstances(gce.projectID, prefix, statuses...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var insts []Instance
	for _, rawInst := range rawInsts {
		inst := newInstance(rawInst, nil)
		insts = append(insts, *inst)
	}
	return insts, nil
}

// removeInstance sends a request to the GCE API to remove the instance
// with the provided ID (in the specified zone). The call blocks until
// the instance is removed (or the request fails).
func (gce *Connection) removeInstance(id, zone string) error {
	err := gce.service.RemoveInstance(gce.projectID, zone, id)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		// TODO(ericsnow) Try removing the firewall anyway?
		return errors.Trace(err)
	}

	fwname := id
	err = gce.service.RemoveFirewall(gce.projectID, fwname)
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
func (gce *Connection) RemoveInstances(prefix string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := gce.Instances(prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	// TODO(ericsnow) Remove instances in parallel?
	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.ID == instID {
				zoneName := path.Base(inst.InstanceSummary.ZoneName)
				if err := gce.removeInstance(instID, zoneName); err != nil {
					failed = append(failed, instID)
					logger.Errorf(context.TODO(), "while removing instance %q: %v", instID, err)
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
func (gce *Connection) UpdateMetadata(key, value string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := gce.service.ListInstances(gce.projectID, "")
	if err != nil {
		return errors.Annotatef(err, "updating metadata for instances %v", ids)
	}
	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.Name == instID {
				if err := gce.updateInstanceMetadata(inst, key, value); err != nil {
					failed = append(failed, instID)
					logger.Errorf(context.TODO(), "while updating metadata for instance %q (%v=%q): %v",
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

func (gce *Connection) updateInstanceMetadata(instance *compute.Instance, key, value string) error {
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
	return errors.Trace(gce.service.SetMetadata(gce.projectID, zoneName, instance.Name, metadata))
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
