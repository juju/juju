// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

// NetworkInterface holds the values for the hook context.
type NetworkInterface struct {
	PublicAddress  string
	PrivateAddress string
	Ports          []network.PortRange
}

// CheckPorts checks the current ports.
func (ni *NetworkInterface) CheckPorts(c *gc.C, expected []network.PortRange) {
	c.Check(ni.Ports, jc.DeepEquals, expected)
}

// AddPorts adds the specified port range.
func (ni *NetworkInterface) AddPorts(protocol string, from, to int) {
	ni.Ports = append(ni.Ports, network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	})
	network.SortPortRanges(ni.Ports)
}

// RemovePorts removes the specified port range.
func (ni *NetworkInterface) RemovePorts(protocol string, from, to int) {
	portRange := network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	}
	for i, port := range ni.Ports {
		if port == portRange {
			ni.Ports = append(ni.Ports[:i], ni.Ports[i+1:]...)
			break
		}
	}
	network.SortPortRanges(ni.Ports)
}

// ContextNetworking is a test double for jujuc.ContextNetworking.
type ContextNetworking struct {
	contextBase
	info *NetworkInterface
}

// PublicAddress implements jujuc.ContextNetworking.
func (c *ContextNetworking) PublicAddress() (string, error) {
	c.stub.AddCall("PublicAddress")

	return c.info.PublicAddress, c.stub.NextErr()

}

// PrivateAddress implements jujuc.ContextNetworking.
func (c *ContextNetworking) PrivateAddress() (string, error) {
	c.stub.AddCall("PrivateAddress")

	return c.info.PrivateAddress, c.stub.NextErr()

}

// OpenPorts implements jujuc.ContextNetworking.
func (c *ContextNetworking) OpenPorts(protocol string, from, to int) error {
	c.stub.AddCall("OpenPorts", protocol, from, to)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.AddPorts(protocol, from, to)
	return nil
}

// ClosePorts implements jujuc.ContextNetworking.
func (c *ContextNetworking) ClosePorts(protocol string, from, to int) error {
	c.stub.AddCall("ClosePorts", protocol, from, to)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.RemovePorts(protocol, from, to)
	return nil
}

// OpenedPorts implements jujuc.ContextNetworking.
func (c *ContextNetworking) OpenedPorts() []network.PortRange {
	c.stub.AddCall("OpenedPorts")
	c.stub.NextErr()

	return c.info.Ports
}
