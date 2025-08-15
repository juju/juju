// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const diskTypesBase = "https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s"

// These are attempt strategies used in waitOperation.
var (
	// TODO(ericsnow) Tune the timeouts and delays.
	longRetryStrategy = retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       2 * time.Second,
		MaxDuration: 5 * time.Minute,
	}
)

func convertRawAPIError(err error) error {
	if err2, ok := err.(*googleapi.Error); ok {
		if err2.Code == http.StatusNotFound {
			return errors.NewNotFound(err, "")
		}
	}
	return err
}

type rawConn struct {
	*compute.Service
}

func (rc *rawConn) GetProjectServiceAccount(projectID string) (string, error) {
	call := rc.Projects.Get(projectID).Fields("defaultServiceAccount")
	proj, err := call.Do()
	if err != nil {
		return "", errors.Trace(err)
	}
	return proj.DefaultServiceAccount, nil
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
	err = rc.waitOperation(projectID, operation, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

func (rc *rawConn) RemoveInstance(projectID, zone, id string) error {
	call := rc.Instances.Delete(projectID, zone, id)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = rc.waitOperation(projectID, operation, longRetryStrategy, returnNotFoundOperationErrors)
	return errors.Trace(err)
}

func matchesPrefix(firewallName, namePrefix string) bool {
	return firewallName == namePrefix || strings.HasPrefix(firewallName, namePrefix+"-")
}

func (rc *rawConn) GetFirewalls(projectID, namePrefix string) ([]*compute.Firewall, error) {
	call := rc.Firewalls.List(projectID)
	firewallList, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}

	if len(firewallList.Items) == 0 {
		return nil, errors.NotFoundf("firewall %q", namePrefix)
	}
	var result []*compute.Firewall
	for _, fw := range firewallList.Items {
		if matchesPrefix(fw.Name, namePrefix) {
			result = append(result, fw)
		}
	}
	return result, nil
}

func (rc *rawConn) AddFirewall(projectID string, firewall *compute.Firewall) error {
	call := rc.Firewalls.Insert(projectID, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = rc.waitOperation(projectID, operation, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

func (rc *rawConn) UpdateFirewall(projectID, name string, firewall *compute.Firewall) error {
	call := rc.Firewalls.Update(projectID, name, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = rc.waitOperation(projectID, operation, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

type handleOperationErrors func(operation *compute.Operation) error

func returnNotFoundOperationErrors(operation *compute.Operation) error {
	if operation.Error != nil {
		result := waitError{operation, nil}
		for _, err := range operation.Error.Errors {
			if err.Code == "RESOURCE_NOT_FOUND" {
				result.cause = errors.NotFoundf("%v: resource", err.Message)
				continue
			}
			logger.Errorf("GCE operation error: (%s) %s", err.Code, err.Message)
		}
		return result
	}
	return nil
}

func logOperationErrors(operation *compute.Operation) error {
	if operation.Error != nil {
		for _, err := range operation.Error.Errors {
			logger.Errorf("GCE operation error: (%s) %s", err.Code, err.Message)
		}
		return waitError{operation, nil}
	}
	return nil
}

func (rc *rawConn) RemoveFirewall(projectID, name string) error {
	call := rc.Firewalls.Delete(projectID, name)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(convertRawAPIError(err))
	}

	err = rc.waitOperation(projectID, operation, longRetryStrategy, returnNotFoundOperationErrors)
	return errors.Trace(convertRawAPIError(err))
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

func (rc *rawConn) CreateDisk(project, zone string, spec *compute.Disk) error {
	ds := rc.Service.Disks
	formatDiskType(project, zone, spec)
	call := ds.Insert(project, zone, spec)
	op, err := call.Do()
	if err != nil {
		return errors.Annotate(err, "could not create a new disk")
	}
	return errors.Trace(rc.waitOperation(project, op, longRetryStrategy, logOperationErrors))
}

func (rc *rawConn) ListDisks(project string) ([]*compute.Disk, error) {
	ds := rc.Service.Disks
	call := ds.AggregatedList(project)
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

func (rc *rawConn) RemoveDisk(project, zone, id string) error {
	ds := rc.Disks
	call := ds.Delete(project, zone, id)
	op, err := call.Do()
	if err != nil {
		return errors.Annotatef(err, "could not delete disk %q", id)
	}
	return errors.Trace(rc.waitOperation(project, op, longRetryStrategy, returnNotFoundOperationErrors))
}

func (rc *rawConn) GetDisk(project, zone, id string) (*compute.Disk, error) {
	ds := rc.Disks
	call := ds.Get(project, zone, id)
	disk, err := call.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get disk %q at zone %q in project %q", id, zone, project)
	}
	return disk, nil
}

func (rc *rawConn) SetDiskLabels(project, zone, id, labelFingerprint string, labels map[string]string) error {
	ds := rc.Service.Disks
	call := ds.SetLabels(project, zone, id, &compute.ZoneSetLabelsRequest{
		LabelFingerprint: labelFingerprint,
		Labels:           labels,
	})
	_, err := call.Do()
	return errors.Trace(err)
}

func (rc *rawConn) AttachDisk(project, zone, instanceId string, disk *compute.AttachedDisk) error {
	call := rc.Instances.AttachDisk(project, zone, instanceId, disk)
	_, err := call.Do() // Perhaps return something from the Op
	if err != nil {
		return errors.Annotatef(err, "cannot attach volume into %q", instanceId)
	}
	return nil
}

func (rc *rawConn) DetachDisk(project, zone, instanceId, diskDeviceName string) error {
	call := rc.Instances.DetachDisk(project, zone, instanceId, diskDeviceName)
	_, err := call.Do()
	if err != nil {
		return errors.Annotatef(err, "cannot detach volume from %q", instanceId)
	}
	return nil
}

func (rc *rawConn) InstanceDisks(project, zone, instanceId string) ([]*compute.AttachedDisk, error) {
	instance, err := rc.GetInstance(project, zone, instanceId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get instance %q to list its disks", instanceId)
	}
	return instance.Disks, nil
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

func (err waitError) Cause() error {
	if err.cause != nil {
		return err.cause
	}
	return err
}

func isWaitError(err error) bool {
	_, ok := err.(*waitError)
	return ok
}

type opDoer interface {
	Do(...googleapi.CallOption) (*compute.Operation, error)
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
func (rc *rawConn) waitOperation(projectID string, op *compute.Operation, retryStrategy retry.CallArgs, f handleOperationErrors) error {
	// TODO(perrito666) 2016-05-02 lp:1558657
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)

	retryStrategy.IsFatalError = func(err error) bool {
		return !errors.IsNotProvisioned(err)
	}
	retryStrategy.Func = func() error {
		var err error
		op, err = rc.checkOperation(projectID, op)
		if err != nil {
			return err
		}
		if op.Status == StatusDone {
			return nil
		}
		return errors.NewNotProvisioned(nil, "GCE operation not done yet")
	}
	var err error
	if op.Status != StatusDone {
		err = retry.Call(retryStrategy)
	}

	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		// lp:1558657
		err := errors.Annotatef(err, "timed out after %d seconds", time.Now().Sub(started)/time.Second)
		return waitError{op, err}
	}
	if err != nil {
		return errors.Trace(err)
	}
	if err := f(op); err != nil {
		return err
	}

	logger.Infof("GCE operation %q finished", op.Name)
	return nil
}

// ListMachineTypes returns a list of machines available in the project and zone provided.
func (rc *rawConn) ListMachineTypes(projectID, zone string) (*compute.MachineTypeList, error) {
	op := rc.MachineTypes.List(projectID, zone)
	machines, err := op.Do()
	if err != nil {
		return nil, errors.Annotatef(err, "listing machine types for project %q and zone %q", projectID, zone)
	}
	return machines, nil
}

func (rc *rawConn) SetMetadata(projectID, zone, instanceID string, metadata *compute.Metadata) error {
	call := rc.Instances.SetMetadata(projectID, zone, instanceID, metadata)
	op, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = rc.waitOperation(projectID, op, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

func (rc *rawConn) ListSubnetworks(projectID, region string) ([]*compute.Subnetwork, error) {
	ctx := context.Background()
	call := rc.Subnetworks.List(projectID, region)
	var results []*compute.Subnetwork
	err := call.Pages(ctx, func(page *compute.SubnetworkList) error {
		results = append(results, page.Items...)
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func (rc *rawConn) ListNetworks(projectID string) ([]*compute.Network, error) {
	ctx := context.Background()
	call := rc.Networks.List(projectID)
	var results []*compute.Network
	err := call.Pages(ctx, func(page *compute.NetworkList) error {
		results = append(results, page.Items...)
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}
