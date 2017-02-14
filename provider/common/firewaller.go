// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// Firewaller provides the functionality to firewalls in a cloud.
type Firewaller interface {
	// IngressRules returns the list of open ports on the named firewall.
	IngressRules(fwname string) ([]network.IngressRule, error)

	// OpenPorts opens the specified ports on the named firewall.
	OpenPorts(fwname string, rules ...network.IngressRule) error

	// ClosePorts closes the specified ports on the named firewall.
	ClosePorts(fwname string, rules ...network.IngressRule) error
}

// TODO(ericsnow) A generic implementation will likely look a lot like
// provider/gce/google/conn_network.go.

// NewFirewaller returns a basic default implementation
// of Firewaller.
func NewFirewaller() Firewaller {
	return &notImplementedFirewaller{}
}

type notImplementedFirewaller struct{}

// IngressRules implements Firewaller.
func (notImplementedFirewaller) IngressRules(fwname string) ([]network.IngressRule, error) {
	return nil, errors.NotImplementedf("Rules method")
}

// OpenPorts implements Firewaller.
func (notImplementedFirewaller) OpenPorts(fwname string, rules ...network.IngressRule) error {
	return errors.NotImplementedf("OpenPorts method")
}

// ClosePorts implements Firewaller.
func (notImplementedFirewaller) ClosePorts(fwname string, ports ...network.IngressRule) error {
	return errors.NotImplementedf("ClosePorts method")
}
