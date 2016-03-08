// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// Firewaller provides the functionality to firewalls in a cloud.
type Firewaller interface {
	// Ports returns the list of open ports on the named firewall.
	Ports(fwname string) ([]network.PortRange, error)

	// OpenPorts opens the specified ports on the named firewall.
	OpenPorts(fwname string, ports ...network.PortRange) error

	// ClosePorts closes the specified ports on the named firewall.
	ClosePorts(fwname string, ports ...network.PortRange) error
}

// TODO(ericsnow) A generic implementation will likely look a lot like
// provider/gce/google/conn_network.go.

// NewFirewaller returns a basic default implementation
// of Firewaller.
func NewFirewaller() Firewaller {
	return &notImplementedFirewaller{}
}

type notImplementedFirewaller struct{}

// Ports implements Firewaller.
func (notImplementedFirewaller) Ports(fwname string) ([]network.PortRange, error) {
	return nil, errors.NotImplementedf("Ports method")
}

// OpenPorts implements Firewaller.
func (notImplementedFirewaller) OpenPorts(fwname string, ports ...network.PortRange) error {
	return errors.NotImplementedf("OpenPorts method")
}

// ClosePorts implements Firewaller.
func (notImplementedFirewaller) ClosePorts(fwname string, ports ...network.PortRange) error {
	return errors.NotImplementedf("ClosePorts method")
}
