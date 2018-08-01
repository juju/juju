// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"

	"github.com/altoros/gosigma/data"
)

// A NIC interface represents network interface card instance
type NIC interface {
	// Convert to string
	fmt.Stringer

	// IPv4 configuration
	IPv4() IPv4

	// MAC address
	MAC() string

	// Model of virtual network interface card
	Model() string

	// Runtime returns runtime information for network interface card or nil if stopped
	Runtime() RuntimeNIC

	// Virtual LAN resource
	VLAN() Resource
}

// A nic implements network interface card instance in CloudSigma
type nic struct {
	client *Client
	obj    *data.NIC
}

var _ NIC = nic{}

// String method is used to print values passed as an operand to any format that
// accepts a string or to an unformatted printer such as Print.
func (n nic) String() string {
	return fmt.Sprintf(`{Model: %q, MAC: %q, IPv4: %v, VLAN: %v, Runtime: %v}`,
		n.Model(), n.MAC(), n.IPv4(), n.VLAN(), n.Runtime())
}

// IPv4 configuration
func (n nic) IPv4() IPv4 {
	if n.obj.IPv4 != nil {
		return ipv4{n.client, n.obj.IPv4}
	}
	return nil
}

// MAC address
func (n nic) MAC() string { return n.obj.MAC }

// Model of virtual network interface card
func (n nic) Model() string { return n.obj.Model }

// Runtime returns runtime information for network interface card or nil if stopped
func (n nic) Runtime() RuntimeNIC {
	if n.obj.Runtime != nil {
		return runtimeNIC{n.obj.Runtime}
	}
	return nil
}

// Virtual LAN resource
func (n nic) VLAN() Resource {
	if n.obj.VLAN != nil {
		return resource{n.obj.VLAN}
	}
	return nil
}
