// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// MachineNetwork represents the state of a machine network.
type MachineNetwork struct {
	st  *State
	doc machineNetworkDoc
}

// machineNetworkDoc represents a configured network that a machine
// can be a part of.
type machineNetworkDoc struct {
	// Name is the network's name. It should be one of the machine's
	// included networks.
	Name string `bson:"_id"`
	// CIDR holds the network CIDR in the form 192.168.100.0/24.
	CIDR string
	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks.
	VLANTag int
}

func newMachineNetwork(st *State, doc *machineNetworkDoc) *MachineNetwork {
	return &MachineNetwork{st, *doc}
}

// Name returns the network name.
func (m *MachineNetwork) Name() string {
	return m.doc.Name
}

// CIDR returns the network CIDR (e.g. 192.168.50.0/24).
func (m *MachineNetwork) CIDR() string {
	return m.doc.CIDR
}

// VLANTag returns the network VLAN tag. Its a number between 1 and
// 4094 for VLANs and 0 if the network is not a VLAN.
func (m *MachineNetwork) VLANTag() int {
	return m.doc.VLANTag
}

// IsVLAN returns whether the network is a VLAN (has tag > 0) or a
// normal networl.
func (m *MachineNetwork) IsVLAN() bool {
	return m.doc.VLANTag > 0
}

// Remove deletes the network from state, only if no network
// interfaces are using it.
func (m *MachineNetwork) Remove() error {
	noNICs := bson.D{{"networkname", bson.D{{"$ne", m.doc.Name}}}}
	ops := []txn.Op{{
		C:      m.st.machineNetworks.Name,
		Id:     m.doc.Name,
		Remove: true,
	}, {
		C:      m.st.networkInterfaces.Name,
		Assert: noNICs,
	}}
	err := m.st.runTransaction(ops)
	switch err {
	case mgo.ErrAborted:
		return fmt.Errorf("cannot remove machine network %q with existing interfaces")
	case mgo.ErrNotFound:
		return errors.NotFoundf("machine network %q", m.doc.Name)
	}
	return err
}

// Interfaces returns all network interfaces on the network.
func (m *MachineNetwork) Interfaces() ([]*NetworkInterface, error) {
	docs := []networkInterfaceDoc{}
	sel := bson.D{{"networkname", m.doc.Name}}
	err := m.st.networkInterfaces.Find(bson.D{{"networkname", m.doc.Name}}).All(&docs)
	if err != nil {
		return nil, err
	}
	ifaces := make([]*NetworkInterface, len(docs))
	for i, doc := range docs {
		ifaces[i] = newNetworkInterface(m.st, &doc)
	}
	return ifaces, nil
}
