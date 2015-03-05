// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
)

// rawConnectionWrapper facilitates mocking out the GCE API during tests.
type rawConnectionWrapper interface {
	// GetProject sends a request to the GCE API for info about the
	// specified project. If the project does not exist then an error
	// will be returned.
	GetProject(projectID string) (*compute.Project, error)
	// GetInstance sends a request to the GCE API for info about the
	// specified instance. If the instance does not exist then an error
	// will be returned.
	GetInstance(projectID, id, zone string) (*compute.Instance, error)
	// ListInstances sends a request to the GCE API for a list of all
	// instances in project for which the name starts with the provided
	// prefix. The result is also limited to those instances with one of
	// the specified statuses (if any).
	ListInstances(projectID, prefix string, status ...string) ([]*compute.Instance, error)
	// AddInstance sends a request to GCE to add a new instance to the
	// given project, with the provided instance data. The call blocks
	// until the instance is created or the request fails.
	AddInstance(projectID, zone string, spec *compute.Instance) error
	// RemoveInstance sends a request to the GCE API to remove the instance
	// with the provided ID (in the specified zone). The call blocks until
	// the instance is removed (or the request fails).
	RemoveInstance(projectID, id, zone string) error
	// GetFirewall sends an API request to GCE for the information about
	// the named firewall and returns it. If the firewall is not found,
	// errors.NotFound is returned.
	GetFirewall(projectID, name string) (*compute.Firewall, error)
	// AddFirewall requests GCE to add a firewall with the provided info.
	// If the firewall already exists then an error will be returned.
	// The call blocks until the firewall is added or the request fails.
	AddFirewall(projectID string, firewall *compute.Firewall) error
	// UpdateFirewall requests GCE to update the named firewall with the
	// provided info, overwriting the existing data. If the firewall does
	// not exist then an error will be returned. The call blocks until the
	// firewall is updated or the request fails.
	UpdateFirewall(projectID, name string, firewall *compute.Firewall) error
	// RemoveFirewall removed the named firewall from the project. If it
	// does not exist then this is a noop. The call blocks until the
	// firewall is added or the request fails.
	RemoveFirewall(projectID, name string) error
	// ListAvailabilityZones returns the list of availability zones for a given
	// GCE region. If none are found the the list is empty. Any failure in
	// the low-level request is returned as an error.
	ListAvailabilityZones(projectID, region string) ([]*compute.Zone, error)
}

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
	// TODO(ericsnow) name this something else?
	raw rawConnectionWrapper

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

	raw, err := newRawConnection(auth)
	if err != nil {
		return errors.Trace(err)
	}

	gc.raw = &rawConn{raw}
	return nil
}

var newRawConnection = func(auth Auth) (*compute.Service, error) {
	return auth.newConnection()
}

// VerifyCredentials ensures that the authentication credentials used
// to connect are valid for use in the project and region defined for
// the Connection. If they are not then an error is returned.
func (gc Connection) VerifyCredentials() error {
	if _, err := gc.raw.GetProject(gc.ProjectID); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
func (gc *Connection) AvailabilityZones(region string) ([]AvailabilityZone, error) {
	rawZones, err := gc.raw.ListAvailabilityZones(gc.ProjectID, region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var zones []AvailabilityZone
	for _, rawZone := range rawZones {
		zones = append(zones, AvailabilityZone{rawZone})
	}
	return zones, nil
}
