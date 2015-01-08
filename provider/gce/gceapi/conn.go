// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"path"
	"time"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

var (
	// TODO(ericsnow) Tune the timeouts and delays.
	attemptsLong = utils.AttemptStrategy{
		Total: 300 * time.Second, // 5 minutes
		Delay: 2 * time.Second,
	}
	attemptsShort = utils.AttemptStrategy{
		Total: 60 * time.Second,
		Delay: 1 * time.Second,
	}
)

type Connection struct {
	raw *compute.Service

	Region    string
	ProjectID string
}

func (gc Connection) Validate() error {
	if gc.Region == "" {
		return &config.InvalidConfigValue{Key: OSEnvRegion}
	}
	if gc.ProjectID == "" {
		return &config.InvalidConfigValue{Key: OSEnvProjectID}
	}
	return nil
}

func (gc *Connection) Connect(auth Auth) error {
	if gc.raw != nil {
		return errors.New("connect() failed (already connected)")
	}

	service, err := auth.newConnection()
	if err != nil {
		return errors.Trace(err)
	}

	gc.raw = service
	return nil
}

func (gc Connection) VerifyCredentials() error {
	call := gc.raw.Projects.Get(gc.ProjectID)
	if _, err := call.Do(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type operationDoer interface {
	// Do starts some operation and returns a description of it. If an
	// error is returned then the operation was not initiated.
	Do() (*compute.Operation, error)
}

func (gce *Connection) checkOperation(op *compute.Operation) (*compute.Operation, error) {
	var call operationDoer
	if op.Zone != "" {
		zone := zoneName(op)
		call = gce.raw.ZoneOperations.Get(gce.ProjectID, zone, op.Name)
	} else if op.Region != "" {
		region := path.Base(op.Region)
		call = gce.raw.RegionOperations.Get(gce.ProjectID, region, op.Name)
	} else {
		call = gce.raw.GlobalOperations.Get(gce.ProjectID, op.Name)
	}

	updated, err := call.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "request for GCE operation %q failed", op.Name)
	}
	return updated, nil
}

func (gce *Connection) waitOperation(op *compute.Operation, attempts utils.AttemptStrategy) error {
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)
	for a := attempts.Start(); a.Next(); {
		if op.Status == StatusDone {
			break
		}

		var err error
		op, err = gce.checkOperation(op)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if op.Status != StatusDone {
		msg := "GCE operation %q failed: timed out after %d seconds"
		return errors.Errorf(msg, op.Name, time.Now().Sub(started)/time.Second)
	}
	if op.Error != nil {
		for _, err := range op.Error.Errors {
			logger.Errorf("GCE operation error: (%s) %s", err.Code, err.Message)
		}
		return errors.Errorf("GCE operation %q failed", op.Name)
	}

	logger.Infof("GCE operation %q finished", op.Name)
	return nil
}

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

func (gce *Connection) AvailabilityZones(region string) ([]AvailabilityZone, error) {
	call := gce.raw.Zones.List(gce.ProjectID)
	if region != "" {
		call = call.Filter("name eq " + region + "-.*")
	}
	// TODO(ericsnow) Add a timeout?
	var results []AvailabilityZone
	for {
		rawResult, err := call.Do()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, raw := range rawResult.Items {
			results = append(results, AvailabilityZone{raw})
		}

		if rawResult.NextPageToken == "" {
			break
		}
		call = call.PageToken(rawResult.NextPageToken)
	}

	return results, nil
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

func (gce *Connection) firewall(name string) (*compute.Firewall, error) {
	call := gce.raw.Firewalls.List(gce.ProjectID)
	call = call.Filter("name eq " + name)
	firewallList, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	if len(firewallList.Items) == 0 {
		return nil, errors.NotFoundf("firewall %q", name)
	}
	return firewallList.Items[0], nil
}

func (gce *Connection) insertFirewall(firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Insert(gce.ProjectID, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) updateFirewall(name string, firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Update(gce.ProjectID, name, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) deleteFirewall(name string) error {
	call := gce.raw.Firewalls.Delete(gce.ProjectID, name)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce Connection) Ports(fwname string) ([]network.PortRange, error) {
	firewall, err := gce.firewall(fwname)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var ports []network.PortRange
	for _, allowed := range firewall.Allowed {
		for _, portRangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(portRangeStr)
			if err != nil {
				return ports, errors.Annotate(err, "bad ports from GCE")
			}
			portRange.Protocol = allowed.IPProtocol
			ports = append(ports, *portRange)
		}
	}

	return ports, nil
}

func (gce Connection) OpenPorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)

	// Send the request, depending on the current ports.
	if currentPortsSet.IsEmpty() {
		firewall := firewallSpec(name, inputPortsSet)
		if err := gce.insertFirewall(firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

func (gce Connection) ClosePorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)
	newPortsSet := currentPortsSet.Difference(inputPortsSet)

	// Send the request, depending on the current ports.
	if newPortsSet.IsEmpty() {
		if err := gce.deleteFirewall(name); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}
