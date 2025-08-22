// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/retry"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	transporthttp "google.golang.org/api/transport/http"
)

// Connection provides methods for interacting with the GCE API. The
// methods are limited to those needed by the juju GCE provider.
//
// Before calling any of the methods, the Connect method should be
// called to authenticate and open the raw connection to the GCE API.
// Otherwise a panic will result.
type Connection struct {
	*compute.Service
	projectID string
}

// Connect authenticates using the provided credentials and opens a
// low-level connection to the GCE API for the Connection. Calling
// Connect after a successful connection has already been made will
// result in an error. All errors that happen while authenticating and
// connecting are returned by Connect.
func Connect(ctx context.Context, connCfg ConnectionConfig, creds *Credentials) (*Connection, error) {
	service, err := newComputeService(ctx, creds, connCfg.HTTPClient)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Connection{
		Service:   service,
		projectID: connCfg.ProjectID,
	}
	return conn, nil
}

// newComputeService opens a new low-level connection to the GCE API using
// the input credentials and returns it.
// This includes building the OAuth-wrapping network transport.
func newComputeService(ctx context.Context, creds *Credentials, httpClient *jujuhttp.Client) (*compute.Service, error) {
	cfg, err := newJWTConfig(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We're substituting the transport, with a wrapped GCE specific version of
	// the original http.Client.
	newClient := *httpClient.Client()

	tsOpt := option.WithTokenSource(cfg.TokenSource(ctx))
	if newClient.Transport, err = transporthttp.NewTransport(ctx, newClient.Transport, tsOpt); err != nil {
		return nil, errors.Trace(err)
	}

	service, err := compute.NewService(ctx,
		tsOpt,
		option.WithHTTPClient(&newClient),
	)
	return service, errors.Trace(err)
}

func (c *Connection) getProjectServiceAccount(ctx context.Context, projectID string) (string, error) {
	call := c.Projects.Get(projectID).Fields("defaultServiceAccount").
		Context(ctx)
	proj, err := call.Do()
	if err != nil {
		return "", errors.Trace(err)
	}
	return proj.DefaultServiceAccount, nil
}

// VerifyCredentials ensures that the authentication credentials used
// to connect are valid for use in the project and region defined for
// the Connection. If they are not then an error is returned.
func (c *Connection) VerifyCredentials(ctx context.Context) error {
	if _, err := c.getProjectServiceAccount(ctx, c.projectID); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

// DefaultServiceAccount returns the default service account for the project.
func (c *Connection) DefaultServiceAccount(ctx context.Context) (string, error) {
	return c.getProjectServiceAccount(ctx, c.projectID)
}

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
func (c *Connection) checkOperation(projectID string, op *compute.Operation) (*compute.Operation, error) {
	var call opDoer
	if op.Zone != "" {
		zoneName := path.Base(op.Zone)
		call = c.ZoneOperations.Get(projectID, zoneName, op.Name)
	} else if op.Region != "" {
		region := path.Base(op.Region)
		call = c.RegionOperations.Get(projectID, region, op.Name)
	} else {
		call = c.GlobalOperations.Get(projectID, op.Name)
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
func (c *Connection) waitOperation(projectID string, op *compute.Operation, retryStrategy retry.CallArgs, f handleOperationErrors) error {
	// TODO(perrito666) 2016-05-02 lp:1558657
	started := time.Now()
	logger.Infof("GCE operation %q, waiting...", op.Name)

	retryStrategy.IsFatalError = func(err error) bool {
		return !errors.IsNotProvisioned(err)
	}
	retryStrategy.Func = func() error {
		var err error
		op, err = c.checkOperation(projectID, op)
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
