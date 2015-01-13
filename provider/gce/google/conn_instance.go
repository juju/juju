// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
)

// instance sends a request to GCE for a low-level snapshot of data
// about an instance and returns it.
func (gce *Connection) instance(zone, id string) (*compute.Instance, error) {
	svc := Services{
		InstanceGet: gce.raw.Instances.Get(gce.ProjectID, zone, id),
	}

	result, err := doCall(svc)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst, ok := result.(*compute.Instance)
	if !ok {
		return nil, errors.New("unable to convert result to compute.Instance")
	}
	return inst, nil
}

// addInstance sends a request to GCE to add a new instance to the
// connection's project, with the provided instance data and machine
// type. Each of the provided zones is attempted and the first available
// zone is where the instance is provisioned. If no zones are available
// then an error is returned. The instance that was passed in is updated
// with the new instance's data upon success. The call blocks until the
// instance is created or the request fails.
func (gce *Connection) addInstance(inst *compute.Instance, machineType string, zones []string) error {
	for _, zoneName := range zones {
		inst.MachineType = resolveMachineType(zoneName, machineType)
		call := gce.raw.Instances.Insert(
			gce.ProjectID,
			zoneName,
			inst,
		)
		svc := Services{InstanceInsert: call}
		result, err := doCall(svc)
		if err != nil {
			// We are guaranteed the insert failed at the point.
			return errors.Annotate(err, "sending new instance request")
		}

		operation, ok := result.(*compute.Operation)
		if !ok {
			return errors.New("unable to convert result to compute.Operation")
		}

		waitErr := gce.waitOperation(operation, attemptsLong)

		// Check if the instance was created.
		realized, err := rawInstance(gce, zoneName, inst.Name)
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

var rawInstance = func(conn *Connection, zone, id string) (*compute.Instance, error) {
	return conn.instance(zone, id)
}

// Instances sends a request to the GCE API for a list of all instances
// (in the Connection's project) for which the name starts with the
// provided prefix. The result is also limited to those instances with
// one of the specified statuses (if any).
func (gce *Connection) Instances(prefix string, statuses ...string) ([]Instance, error) {
	call := gce.raw.Instances.AggregatedList(gce.ProjectID)
	call = call.Filter("name eq " + prefix + ".*")
	svc := Services{InstanceList: call}

	// TODO(ericsnow) Add a timeout?
	var results []Instance
	for {
		result, err := doCall(svc)
		if err != nil {
			return results, errors.Trace(err)
		}

		raw, ok := result.(*compute.InstanceAggregatedList)
		if !ok {
			return nil, errors.New("unable to convert result to compute.InstanceList")
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
		svc.InstanceList = instsNextPage(call, raw.NextPageToken)
	}

	return filterInstances(results, statuses...), nil
}

var instsNextPage = func(call *compute.InstancesAggregatedListCall, token string) *compute.InstancesAggregatedListCall {
	return call.PageToken(token)
}

// removeInstance sends a request to the GCE API to remove the instance
// with the provided ID (in the specified zone). The call blocks until
// the instance is removed (or the request fails).
func (gce *Connection) removeInstance(id, zone string) error {
	svc := Services{
		InstanceDelete: gce.raw.Instances.Delete(gce.ProjectID, zone, id),
	}

	result, err := doCall(svc)
	if err != nil {
		return errors.Trace(err)
	}
	operation, ok := result.(*compute.Operation)
	if !ok {
		return errors.New("unable to convert result to compute.Operation")
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}

	if err := connRemoveFirewall(gce, id); err != nil {
		return errors.Trace(err)
	}

	return nil
}

var connRemoveFirewall = func(conn *Connection, fwname string) error {
	return conn.deleteFirewall(fwname)
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
