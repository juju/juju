// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
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
		Total: 300 * time.Second, // 5 minutes
		Delay: 2 * time.Second,
	}
	attemptsShort = utils.AttemptStrategy{
		Total: 60 * time.Second,
		Delay: 1 * time.Second,
	}
)

// TODO(ericsnow) Add specific error types for common failures
// (e.g. BadRequest, RequestFailed, RequestError, ConnectionFailed)?

// Connection provides methods for interacting with the GCE API. The
// methods are limited to those needed by the juju GCE provider.
//
// Before calling any of the methods, the Connect method should be
// called to authenticate and open the raw connection to the GCE API.
// Otherwise a panic will result.
//
// Both Region and ProjectID should be set on the connection before
// using it. Use ValidateConnection before using the connection to
// ensure this is the case.
type Connection struct {
	raw *compute.Service

	// Region is the GCE region in which to operate for this connection.
	Region string
	// ProjectID is the project ID to use in all GCE API requests for
	// this connection.
	ProjectID string
}

// Services holds pointers to GCE endpoints we call
type Services struct {
	ZoneList *compute.ZonesListCall

	ZoneOp   *compute.ZoneOperationsGetCall
	RegionOp *compute.RegionOperationsGetCall
	GlobalOp *compute.GlobalOperationsGetCall

	InstanceList   *compute.InstancesAggregatedListCall
	InstanceGet    *compute.InstancesGetCall
	InstanceInsert *compute.InstancesInsertCall
	InstanceDelete *compute.InstancesDeleteCall

	ProjectGet *compute.ProjectsGetCall

	FirewallList   *compute.FirewallsListCall
	FirewallInsert *compute.FirewallsInsertCall
	FirewallUpdate *compute.FirewallsUpdateCall
	FirewallDelete *compute.FirewallsDeleteCall
}

// doCall works through potential services of a Services struct.
// If it finds a non-empty pointer it returns the response of the
// Do method for service call.
var doCall = func(svc Services) (interface{}, error) {
	if svc.ZoneOp != nil {
		return svc.ZoneOp.Do()
	} else if svc.ZoneList != nil {
		return svc.ZoneList.Do()
	} else if svc.RegionOp != nil {
		return svc.RegionOp.Do()
	} else if svc.GlobalOp != nil {
		return svc.GlobalOp.Do()
	} else if svc.ProjectGet != nil {
		return svc.ProjectGet.Do()
	} else if svc.FirewallList != nil {
		return svc.FirewallList.Do()
	}
	return nil, errors.New("no suitable service found")
}

// TODO(ericsnow) Verify in each method that Connection.raw is set?

// Connect authenticates using the provided credentials and opens a
// low-level connection to the GCE API for the Connection. Calling
// Connect after a successful connection has already been made will
// result in an error. All errors that happen while authenticating and
// connecting are returned by Connect.
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

// VerifyCredentials ensures that the authentication credentials used
// to connect are valid for use in the project and region defined for
// the Connection. If they are not then an error is returned.
func (gc Connection) VerifyCredentials() error {
	call := gc.raw.Projects.Get(gc.ProjectID)
	svc := Services{ProjectGet: call}
	if _, err := doCall(svc); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

// checkOperation requests a new copy of the given operation from the
// GCE API and returns it. The new copy will have the operation's
// current status.
func (gc *Connection) checkOperation(op *compute.Operation) (*compute.Operation, error) {
	svc := Services{}

	if op.Zone != "" {
		zone := zoneName(op)
		svc.ZoneOp = gc.raw.ZoneOperations.Get(gc.ProjectID, zone, op.Name)
	} else if op.Region != "" {
		region := path.Base(op.Region)
		svc.RegionOp = gc.raw.RegionOperations.Get(gc.ProjectID, region, op.Name)
	} else {
		svc.GlobalOp = gc.raw.GlobalOperations.Get(gc.ProjectID, op.Name)
	}

	result, err := doCall(svc)
	if err != nil {
		return nil, errors.Annotatef(err, "request for GCE operation %q failed", op.Name)
	}

	operation, ok := result.(*compute.Operation)
	if !ok {
		return nil, errors.Annotatef(err, "unable to cast result to compute.Operation")
	}
	return operation, nil
}

// waitOperation waits for the provided operation to reach the "done"
// status. It follows the given attempt strategy (e.g. wait time between
// attempts) and may time out.
func (gc *Connection) waitOperation(op *compute.Operation, attempts utils.AttemptStrategy) error {
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)
	for a := attempts.Start(); a.Next(); {
		if op.Status == StatusDone {
			break
		}

		var err error
		op, err = gc.checkOperation(op)
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

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
func (gc *Connection) AvailabilityZones(region string) ([]AvailabilityZone, error) {
	call := gc.raw.Zones.List(gc.ProjectID)
	if region != "" {
		call = call.Filter("name eq " + region + "-.*")
	}

	// TODO(ericsnow) Add a timeout?
	svc := Services{ZoneList: call}
	var results []AvailabilityZone
	for {
		result, err := doCall(svc)
		if err != nil {
			return nil, errors.Trace(err)
		}

		rawResult, ok := result.(compute.ZoneList)
		if !ok {
			return nil, errors.Annotatef(err, "unable to cast result to compute.ZoneList")
		}

		for _, raw := range rawResult.Items {
			results = append(results, AvailabilityZone{raw})
		}

		if rawResult.NextPageToken == "" {
			break
		}
		svc.ZoneList = call.PageToken(rawResult.NextPageToken)
	}

	return results, nil
}
