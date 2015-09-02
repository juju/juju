// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
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
	// CreateDisk will create a gce Persistent Block device that matches
	// the specified in spec.
	CreateDisk(project, zone string, spec *compute.Disk) error
	// ListDisks returns a list of disks available for a given project.
	ListDisks(project, zone string) ([]*compute.Disk, error)
	// RemoveDisk will delete the disk identified by id.
	RemoveDisk(project, zone, id string) error
	// GetDisk will return the disk correspondent to the passed id.
	GetDisk(project, zone, id string) (*compute.Disk, error)
	// AttachDisk will attach the disk described in attachedDisks (if it exists) into
	// the instance with id instanceId.
	AttachDisk(project, zone, instanceId string, attachedDisk *compute.AttachedDisk) error
	// Detach disk detaches device diskDeviceName (if it exists and its attached)
	// form the machine with id instanceId.
	DetachDisk(project, zone, instanceId, diskDeviceName string) error
	// InstanceDisks returns the disks attached to the instance identified
	// by instanceId
	InstanceDisks(project, zone, instanceId string) ([]*compute.AttachedDisk, error)
}

// TODO(ericsnow) Add specific error types for common failures
// (e.g. BadRequest, RequestFailed, RequestError, ConnectionFailed)?

// Connection provides methods for interacting with the GCE API. The
// methods are limited to those needed by the juju GCE provider.
//
// Before calling any of the methods, the Connect method should be
// called to authenticate and open the raw connection to the GCE API.
// Otherwise a panic will result.
type Connection struct {
	// TODO(ericsnow) name this something else?
	raw       rawConnectionWrapper
	region    string
	projectID string
}

// Connect authenticates using the provided credentials and opens a
// low-level connection to the GCE API for the Connection. Calling
// Connect after a successful connection has already been made will
// result in an error. All errors that happen while authenticating and
// connecting are returned by Connect.
func Connect(connCfg ConnectionConfig, creds *Credentials) (*Connection, error) {
	raw, err := newRawConnection(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Connection{
		raw:       &rawConn{raw},
		region:    connCfg.Region,
		projectID: connCfg.ProjectID,
	}
	return conn, nil
}

var newRawConnection = func(creds *Credentials) (*compute.Service, error) {
	return newConnection(creds)
}

// TODO(ericsnow) Verify in each method that Connection.raw is set?

// VerifyCredentials ensures that the authentication credentials used
// to connect are valid for use in the project and region defined for
// the Connection. If they are not then an error is returned.
func (gc Connection) VerifyCredentials() error {
	if _, err := gc.raw.GetProject(gc.projectID); err != nil {
		// TODO(ericsnow) Wrap err with something about bad credentials?
		return errors.Trace(err)
	}
	return nil
}

// AvailabilityZones returns the list of availability zones for a given
// GCE region. If none are found the the list is empty. Any failure in
// the low-level request is returned as an error.
func (gc *Connection) AvailabilityZones(region string) ([]AvailabilityZone, error) {
	rawZones, err := gc.raw.ListAvailabilityZones(gc.projectID, region)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var zones []AvailabilityZone
	for _, rawZone := range rawZones {
		zones = append(zones, AvailabilityZone{rawZone})
	}
	return zones, nil
}
