// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
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
func (c *Connection) Firewalls(ctx context.Context, prefix string) ([]*computepb.Firewall, error) {
	filter := fmt.Sprintf("name eq '^%s(-.+)?$'", prefix)
	iter := c.firewalls.List(ctx, &computepb.ListFirewallsRequest{
		Project: c.projectID,
		Filter:  &filter,
	})
	return fetchResults[computepb.Firewall](iter.Next, "firewalls")
}

// AddFirewall adds a new firewall to the project.
func (c *Connection) AddFirewall(ctx context.Context, firewall *computepb.Firewall) error {
	op, err := c.firewalls.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          c.projectID,
		FirewallResource: firewall,
	})
	if err == nil {
		err = op.Wait(ctx)
	}

	return errors.Annotatef(err, "adding firewall %q", firewall.GetName())
}

// UpdateFirewall updates an existing firewall.
func (c *Connection) UpdateFirewall(ctx context.Context, name string, firewall *computepb.Firewall) error {
	op, err := c.firewalls.Update(ctx, &computepb.UpdateFirewallRequest{
		Project:          c.projectID,
		Firewall:         name,
		FirewallResource: firewall,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "updating firewall %q", name)
}

// RemoveFirewall removes the named firewall from the project.
func (c *Connection) RemoveFirewall(ctx context.Context, fwname string) (err error) {
	defer func() {
		if IsNotFound(err) {
			err = nil
		}
	}()
	op, err := c.firewalls.Delete(ctx, &computepb.DeleteFirewallRequest{
		Project:  c.projectID,
		Firewall: fwname,
	})
	if err == nil {
		err = op.Wait(ctx)
	}
	return errors.Annotatef(err, "deleting firewall %q", fwname)
}

// Subnetworks returns the subnets available in this region.
func (c *Connection) Subnetworks(ctx context.Context, region string) ([]*computepb.Subnetwork, error) {
	iter := c.subnetworks.List(ctx, &computepb.ListSubnetworksRequest{
		Project: c.projectID,
		Region:  region,
	})
	return fetchResults[computepb.Subnetwork](iter.Next, "subnets")
}

// Networks returns the networks available.
func (c *Connection) Networks(ctx context.Context) ([]*computepb.Network, error) {
	iter := c.networks.List(ctx, &computepb.ListNetworksRequest{
		Project: c.projectID,
	})
	return fetchResults[computepb.Network](iter.Next, "networks")
}
