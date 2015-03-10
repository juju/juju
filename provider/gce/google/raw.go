// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"path"
	"time"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/utils"
)

// These are attempt strategies used in waitOperation.
var (
	// TODO(ericsnow) Tune the timeouts and delays.

	attemptsLong = utils.AttemptStrategy{
		Total: 5 * time.Minute,
		Delay: 2 * time.Second,
	}
	attemptsShort = utils.AttemptStrategy{
		Total: 1 * time.Minute,
		Delay: 1 * time.Second,
	}
)

type rawConn struct {
	*compute.Service
}

func (rc *rawConn) GetProject(projectID string) (*compute.Project, error) {
	call := rc.Projects.Get(projectID)
	proj, err := call.Do()
	return proj, errors.Trace(err)
}

func (rc *rawConn) GetInstance(projectID, zone, id string) (*compute.Instance, error) {
	call := rc.Instances.Get(projectID, zone, id)
	inst, err := call.Do()
	return inst, errors.Trace(err)
}

func (rc *rawConn) ListInstances(projectID, prefix string, statuses ...string) ([]*compute.Instance, error) {
	call := rc.Instances.AggregatedList(projectID)
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

func (rc *rawConn) AddInstance(projectID, zoneName string, spec *compute.Instance) error {
	call := rc.Instances.Insert(projectID, zoneName, spec)
	operation, err := call.Do()
	if err != nil {
		// We are guaranteed the insert failed at the point.
		return errors.Annotate(err, "sending new instance request")
	}

	err = rc.waitOperation(projectID, operation, attemptsLong)
	return errors.Trace(err)
}

func (rc *rawConn) RemoveInstance(projectID, zone, id string) error {
	call := rc.Instances.Delete(projectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}

	err = rc.waitOperation(projectID, operation, attemptsLong)
	return errors.Trace(err)
}

func (rc *rawConn) GetFirewall(projectID, name string) (*compute.Firewall, error) {
	call := rc.Firewalls.List(projectID)
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

func (rc *rawConn) AddFirewall(projectID string, firewall *compute.Firewall) error {
	call := rc.Firewalls.Insert(projectID, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}

	err = rc.waitOperation(projectID, operation, attemptsLong)
	return errors.Trace(err)
}

func (rc *rawConn) UpdateFirewall(projectID, name string, firewall *compute.Firewall) error {
	call := rc.Firewalls.Update(projectID, name, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}

	err = rc.waitOperation(projectID, operation, attemptsLong)
	return errors.Trace(err)
}

func (rc *rawConn) RemoveFirewall(projectID, name string) error {
	call := rc.Firewalls.Delete(projectID, name)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}

	err = rc.waitOperation(projectID, operation, attemptsLong)
	return errors.Trace(err)
}

func (rc *rawConn) ListAvailabilityZones(projectID, region string) ([]*compute.Zone, error) {
	call := rc.Zones.List(projectID)
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

type waitError struct {
	op    *compute.Operation
	cause error
}

func (err waitError) Error() string {
	if err.cause != nil {
		return fmt.Sprintf("GCE operation %q failed: %v", err.op.Name, err.cause)
	}
	return fmt.Sprintf("GCE operation %q failed", err.op.Name)
}

func isWaitError(err error) bool {
	_, ok := err.(*waitError)
	return ok
}

type opDoer interface {
	Do() (*compute.Operation, error)
}

// checkOperation requests a new copy of the given operation from the
// GCE API and returns it. The new copy will have the operation's
// current status.
func (rc *rawConn) checkOperation(projectID string, op *compute.Operation) (*compute.Operation, error) {
	var call opDoer
	if op.Zone != "" {
		zoneName := path.Base(op.Zone)
		call = rc.ZoneOperations.Get(projectID, zoneName, op.Name)
	} else if op.Region != "" {
		region := path.Base(op.Region)
		call = rc.RegionOperations.Get(projectID, region, op.Name)
	} else {
		call = rc.GlobalOperations.Get(projectID, op.Name)
	}

	operation, err := doOpCall(call)
	if err != nil {
		return nil, errors.Annotatef(err, "request for GCE operation %q failed", op.Name)
	}
	return operation, nil
}

var doOpCall = func(call opDoer) (*compute.Operation, error) {
	return call.Do()
}

// waitOperation waits for the provided operation to reach the "done"
// status. It follows the given attempt strategy (e.g. wait time between
// attempts) and may time out.
func (rc *rawConn) waitOperation(projectID string, op *compute.Operation, attempts utils.AttemptStrategy) error {
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)
	for a := attempts.Start(); a.Next(); {
		if op.Status == StatusDone {
			break
		}

		var err error
		op, err = rc.checkOperation(projectID, op)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if op.Status != StatusDone {
		err := errors.Errorf("timed out after %d seconds", time.Now().Sub(started)/time.Second)
		return waitError{op, err}
	}
	if op.Error != nil {
		for _, err := range op.Error.Errors {
			logger.Errorf("GCE operation error: (%s) %s", err.Code, err.Message)
		}
		return waitError{op, nil}
	}

	logger.Infof("GCE operation %q finished", op.Name)
	return nil
}
