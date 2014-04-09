// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/names"
)

// Network represents the state of a network.
type Network struct {
	st  *State
	doc networkDoc
}

// networkDoc represents a configured network that a machine can be a
// part of.
type networkDoc struct {
	// Id is the network's id. It should be one of the machine's
	// included networks. This is intended to be a juju-specific id
	// and to have a separate field for provider-specific name, but
	// for now both are the same.
	Id string `bson:"_id"`
	// CIDR holds the network CIDR in the form 192.168.100.0/24.
	CIDR string
	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks.
	VLANTag int
}

func newNetwork(st *State, doc *networkDoc) *Network {
	return &Network{st, *doc}
}

// Id returns the network id.
func (n *Network) Id() string {
	return n.doc.Id
}

// Tag returns the network tag.
func (n *Network) Tag() string {
	return names.NetworkTag(n.doc.Id)
}

// CIDR returns the network CIDR (e.g. 192.168.50.0/24).
func (n *Network) CIDR() string {
	return n.doc.CIDR
}

// VLANTag returns the network VLAN tag. Its a number between 1 and
// 4094 for VLANs and 0 if the network is not a VLAN.
func (n *Network) VLANTag() int {
	return n.doc.VLANTag
}

// IsVLAN returns whether the network is a VLAN (has tag > 0) or a
// normal network.
func (n *Network) IsVLAN() bool {
	return n.doc.VLANTag > 0
}

// Interfaces returns all network interfaces on the network.
func (n *Network) Interfaces() ([]*NetworkInterface, error) {
	docs := []networkInterfaceDoc{}
	sel := bson.D{{"networkid", n.doc.Id}}
	err := n.st.networkInterfaces.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	ifaces := make([]*NetworkInterface, len(docs))
	for i, doc := range docs {
		ifaces[i] = newNetworkInterface(n.st, &doc)
	}
	return ifaces, nil
}
