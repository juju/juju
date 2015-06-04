// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
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

// ContextNetworking is a test double for jujuc.ContextNetworking.
type ContextNetworking struct {
	Stub *testing.Stub
	Info *NetworkInterface
}

func (c *ContextNetworking) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Info == nil {
		c.Info = &NetworkInterface{}
	}
}

// CheckPorts checks the current ports.
func (cn *ContextNetworking) CheckPorts(c *gc.C, expected []network.PortRange) {
	cn.init()
	c.Check(cn.Info.Ports, jc.DeepEquals, expected)
}

// PublicAddress implements jujuc.ContextNetworking.
func (cn *ContextNetworking) PublicAddress() (string, bool) {
	cn.Stub.AddCall("PublicAddress")
	cn.Stub.NextErr()
	cn.init()
	if cn.Info.PublicAddress == "" {
		return "", false
	}
	return cn.Info.PublicAddress, true
}

// PrivateAddress implements jujuc.ContextNetworking.
func (cn *ContextNetworking) PrivateAddress() (string, bool) {
	cn.Stub.AddCall("PrivateAddress")
	cn.Stub.NextErr()
	cn.init()
	if cn.Info.PrivateAddress == "" {
		return "", false
	}
	return cn.Info.PrivateAddress, true
}

// OpenPorts implements jujuc.ContextNetworking.
func (cn *ContextNetworking) OpenPorts(protocol string, from, to int) error {
	cn.Stub.AddCall("OpenPorts", protocol, from, to)
	if err := cn.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	cn.init()
	cn.Info.Ports = append(cn.Info.Ports, network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	})
	network.SortPortRanges(cn.Info.Ports)
	return nil
}

// ClosePorts implements jujuc.ContextNetworking.
func (cn *ContextNetworking) ClosePorts(protocol string, from, to int) error {
	cn.Stub.AddCall("ClosePorts", protocol, from, to)
	if err := cn.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	cn.init()
	portRange := network.PortRange{
		Protocol: protocol,
		FromPort: from,
		ToPort:   to,
	}
	for i, port := range cn.Info.Ports {
		if port == portRange {
			cn.Info.Ports = append(cn.Info.Ports[:i], cn.Info.Ports[i+1:]...)
			break
		}
	}
	network.SortPortRanges(cn.Info.Ports)
	return nil
}

// OpenedPorts implements jujuc.ContextNetworking.
func (cn *ContextNetworking) OpenedPorts() []network.PortRange {
	cn.Stub.AddCall("OpenedPorts")
	cn.Stub.NextErr()
	cn.init()
	return cn.Info.Ports
}
