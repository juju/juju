// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"path"
	"time"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/utils"
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
