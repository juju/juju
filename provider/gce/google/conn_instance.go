// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
)

func (gce *Connection) instance(zone, id string) (*compute.Instance, error) {
	call := gce.raw.Instances.Get(gce.ProjectID, zone, id)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

func (gce *Connection) addInstance(inst *compute.Instance, machineType string, zones []string) error {
	for _, zoneName := range zones {
		inst.MachineType = resolveMachineType(zoneName, machineType)
		call := gce.raw.Instances.Insert(
			gce.ProjectID,
			zoneName,
			inst,
		)
		operation, err := call.Do()
		if err != nil {
			// We are guaranteed the insert failed at the point.
			return errors.Annotate(err, "sending new instance request")
		}
		waitErr := gce.waitOperation(operation, attemptsLong)

		// Check if the instance was created.
		realized, err := gce.instance(zoneName, inst.Name)
		if err != nil {
			if waitErr == nil {
				return errors.Trace(err)
			}
			// Try the next zone.
			logger.Errorf("failed to get new instance in zone %q: %v", zoneName, waitErr)
			continue
		}

		// Success!
		*inst = *realized
		return nil
	}
	return errors.Errorf("not able to provision in any zone")
}

func (gce *Connection) Instances(prefix string, statuses ...string) ([]Instance, error) {
	call := gce.raw.Instances.AggregatedList(gce.ProjectID)
	call = call.Filter("name eq " + prefix + ".*")

	// TODO(ericsnow) Add a timeout?
	var results []Instance
	for {
		raw, err := call.Do()
		if err != nil {
			return results, errors.Trace(err)
		}

		for _, item := range raw.Items {
			for _, raw := range item.Instances {
				inst := newInstance(raw)
				results = append(results, *inst)
			}
		}

		if raw.NextPageToken == "" {
			break
		}
		call = call.PageToken(raw.NextPageToken)
	}

	return filterInstances(results, statuses...), nil
}

func (gce *Connection) removeInstance(id, zone string) error {
	call := gce.raw.Instances.Delete(gce.ProjectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}

	if err := gce.deleteFirewall(id); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (gce *Connection) RemoveInstances(prefix string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	instances, err := gce.Instances(prefix)
	if err != nil {
		return errors.Annotatef(err, "while removing instances %v", ids)
	}

	var failed []string
	for _, instID := range ids {
		for _, inst := range instances {
			if inst.ID == instID {
				if err := gce.removeInstance(instID, zoneName(inst)); err != nil {
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
