// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

// NetworkInterface holds the values for the hook context.
type NetworkInterface struct {
	PublicAddress        string
	PrivateAddress       string
	PortRangesByEndpoint network.GroupedPortRanges
	NetworkInfoResults   map[string]params.NetworkInfoResult
}

// CheckPorts checks the current ports.
func (ni *NetworkInterface) CheckPortRanges(c *tc.C, expected network.GroupedPortRanges) {
	c.Check(ni.PortRangesByEndpoint, jc.DeepEquals, expected)
}

// AddPortRanges adds the specified port range.
func (ni *NetworkInterface) AddPortRange(endpoint string, portRange network.PortRange) {
	if ni.PortRangesByEndpoint == nil {
		ni.PortRangesByEndpoint = make(network.GroupedPortRanges)
	}
	ni.PortRangesByEndpoint[endpoint] = append(ni.PortRangesByEndpoint[endpoint], portRange)
	network.SortPortRanges(ni.PortRangesByEndpoint[endpoint])
}

// RemovePortRange removes the specified port range.
func (ni *NetworkInterface) RemovePortRange(endpoint string, portRange network.PortRange) {
	if ni.PortRangesByEndpoint == nil {
		return
	}

	for i, existingPortRange := range ni.PortRangesByEndpoint[endpoint] {
		if existingPortRange == portRange {
			ni.PortRangesByEndpoint[endpoint] = append(ni.PortRangesByEndpoint[endpoint][:i], ni.PortRangesByEndpoint[endpoint][i+1:]...)
			break
		}
	}
	network.SortPortRanges(ni.PortRangesByEndpoint[endpoint])
}

// ContextNetworking is a test double for jujuc.ContextNetworking.
type ContextNetworking struct {
	contextBase
	info *NetworkInterface
}

// PublicAddress implements jujuc.ContextNetworking.
func (c *ContextNetworking) PublicAddress(_ context.Context) (string, error) {
	c.stub.AddCall("PublicAddress")

	return c.info.PublicAddress, c.stub.NextErr()

}

// PrivateAddress implements jujuc.ContextNetworking.
func (c *ContextNetworking) PrivateAddress() (string, error) {
	c.stub.AddCall("PrivateAddress")

	return c.info.PrivateAddress, c.stub.NextErr()

}

// OpenPortRange implements jujuc.ContextNetworking.
func (c *ContextNetworking) OpenPortRange(endpoint string, portRange network.PortRange) error {
	c.stub.AddCall("OpenPortRange", endpoint, portRange)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.AddPortRange(endpoint, portRange)
	return nil
}

// ClosePortRange implements jujuc.ContextNetworking.
func (c *ContextNetworking) ClosePortRange(endpoint string, portRange network.PortRange) error {
	c.stub.AddCall("ClosePortRange", endpoint, portRange)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.RemovePortRange(endpoint, portRange)
	return nil
}

// OpenedPortRanges implements jujuc.ContextNetworking.
func (c *ContextNetworking) OpenedPortRanges() network.GroupedPortRanges {
	c.stub.AddCall("OpenedPortRanges")
	_ = c.stub.NextErr()

	return c.info.PortRangesByEndpoint
}

// NetworkInfo implements jujuc.ContextNetworking.
func (c *ContextNetworking) NetworkInfo(_ context.Context, bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	c.stub.AddCall("NetworkInfo", bindingNames, relationId)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return c.info.NetworkInfoResults, nil
}
