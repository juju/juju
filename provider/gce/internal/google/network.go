// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"
)

const (
	NetworkDefaultName = "default"
	NetworkPathRoot    = "global/networks/"
)

// The different kinds of network access.
const (
	NetworkAccessOneToOneNAT = "ONE_TO_ONE_NAT" // the default
)

// Firewalls collects the firewall rules for the given name prefix
// (within the Connection's project) and returns them any matches.
func (c *Connection) Firewalls(ctx context.Context, prefix string) ([]*compute.Firewall, error) {
	filter := fmt.Sprintf("name eq '^%s(-.+)?$'", prefix)
	call := c.Service.Firewalls.List(c.projectID).
		Context(ctx).Filter(filter)
	var results []*compute.Firewall
	err := call.Pages(ctx, func(page *compute.FirewallList) error {
		results = append(results, page.Items...)
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// AddFirewall adds a new firewall to the project.
func (c *Connection) AddFirewall(ctx context.Context, firewall *compute.Firewall) error {
	call := c.Service.Firewalls.Insert(c.projectID, firewall).
		Context(ctx)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = c.waitOperation(c.projectID, operation, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

// UpdateFirewall updates an existing firewall.
func (c *Connection) UpdateFirewall(ctx context.Context, name string, firewall *compute.Firewall) error {
	call := c.Service.Firewalls.Update(c.projectID, name, firewall).
		Context(ctx)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	err = c.waitOperation(c.projectID, operation, longRetryStrategy, logOperationErrors)
	return errors.Trace(err)
}

// RemoveFirewall removes the named firewall from the project.
func (c *Connection) RemoveFirewall(ctx context.Context, fwname string) (err error) {
	defer func() {
		if IsNotFound(err) {
			err = nil
		}
	}()
	call := c.Service.Firewalls.Delete(c.projectID, fwname).
		Context(ctx)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(convertRawAPIError(err))
	}

	err = c.waitOperation(c.projectID, operation, longRetryStrategy, returnNotFoundOperationErrors)
	return errors.Trace(err)
}

// Subnetworks returns the subnets available in this region.
func (c *Connection) Subnetworks(ctx context.Context, region string) ([]*compute.Subnetwork, error) {
	call := c.Service.Subnetworks.List(c.projectID, region).
		Context(ctx)
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

// Networks returns the networks available.
func (c *Connection) Networks(ctx context.Context) ([]*compute.Network, error) {
	call := c.Service.Networks.List(c.projectID).
		Context(ctx)
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
