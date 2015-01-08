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
	if _, err := call.Do(); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

type operationDoer interface {
	// Do starts some operation and returns a description of it. If an
	// error is returned then the operation was not initiated.
	Do() (*compute.Operation, error)
}

// checkOperation requests a new copy of the given operation from the
// GCE API and returns it. The new copy will have the operation's
// current status.
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

// waitOperation waits for the provided operation to reach the "done"
// status. It follows the given attempt strategy (e.g. wait time between
// attempts) and may time out.
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

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
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
