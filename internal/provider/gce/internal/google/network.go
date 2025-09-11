// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

const (
	NetworkDefaultName  = "default"
	NetworkPathRoot     = "global/networks/"
	ExternalNetworkName = "External NAT"
)

// NetworkStatus defines the status of a network.
type NetworkStatus string

const (
	NetworkStatusReady NetworkStatus = "READY"
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
	return fetchResults[computepb.Firewall](iter.All(), "firewalls")
}

// NetworkFirewalls returns the firewalls associated with the specified network.
func (c *Connection) NetworkFirewalls(ctx context.Context, networkURL string) ([]*computepb.Firewall, error) {
	iter := c.firewalls.List(ctx, &computepb.ListFirewallsRequest{
		Project: c.projectID,
	})
	return fetchResults[computepb.Firewall](iter.All(), "network firewalls", func(fw *computepb.Firewall) bool {
		// Unfortunately, we can't filter on network URl so need to do it client side.
		return fw.GetNetwork() == networkURL
	})
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

var allowedSubnetPurposes = set.NewStrings(
	"",
	"PURPOSE_PRIVATE",
)

// Subnetworks returns the subnets available in this region.
func (c *Connection) Subnetworks(ctx context.Context, region string, urls ...string) ([]*computepb.Subnetwork, error) {
	req := &computepb.ListSubnetworksRequest{
		Project: c.projectID,
		Region:  region,
	}
	urlSet := set.NewStrings(urls...)
	wantAll := urlSet.Size() == 0
	iter := c.subnetworks.List(ctx, req)
	return fetchResults[computepb.Subnetwork](iter.All(), "subnetworks", func(subnet *computepb.Subnetwork) bool {
		// Filter out special purpose subnets.
		if wantAll {
			return allowedSubnetPurposes.Contains(subnet.GetPurpose())
		}
		// Unfortunately, we can't filter on self link URl so need to do it client side.
		return urlSet.Contains(subnet.GetSelfLink())
	})
}

// NetworkSubnetworks returns the subnets in the specified network.
func (c *Connection) NetworkSubnetworks(ctx context.Context, region string, networkURL string) ([]*computepb.Subnetwork, error) {
	req := &computepb.ListSubnetworksRequest{
		Project: c.projectID,
		Region:  region,
	}
	iter := c.subnetworks.List(ctx, req)
	return fetchResults[computepb.Subnetwork](iter.All(), "subnetworks", func(subnet *computepb.Subnetwork) bool {
		// Unfortunately, we can't filter on network URl so need to do it client side.
		return subnet.GetNetwork() == networkURL
	})
}

// Networks returns the networks available.
func (c *Connection) Networks(ctx context.Context) ([]*computepb.Network, error) {
	iter := c.networks.List(ctx, &computepb.ListNetworksRequest{
		Project: c.projectID,
	})
	return fetchResults[computepb.Network](iter.All(), "networks")
}

// Network returns the network with the given id.
func (c *Connection) Network(ctx context.Context, id string) (*computepb.Network, error) {
	network, err := c.networks.Get(ctx, &computepb.GetNetworkRequest{
		Project: c.projectID,
		Network: id,
	})
	return network, errors.Annotatef(
		convertError(err, "network", id), "getting network %q", network)
}
