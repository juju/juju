// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// NetworkInterface represents the state of a machine network
// interface.
type NetworkInterface struct {
	st  *State
	doc networkInterfaceDoc
}

// networkInterfaceDoc represents a network interface for a machine on
// a given network.
type networkInterfaceDoc struct {
	MACAddress string `bson:"_id"`
	// Name is the network interface name
	Name        string
	NetworkName string
	MachineId   string
}

func newNetworkInterface(st *State, doc *networkInterfaceDoc) *NetworkInterface {
	return &NetworkInterface{st, *doc}
}

// MACAddress returns the MAC address of the interface.
func (ni *NetworkInterface) MACAddress() string {
	return ni.doc.MACAddress
}

// Name returns the name of the interface.
func (ni *NetworkInterface) Name() string {
	return ni.doc.Name
}

// NetworkName returns the machine network name of the interface.
func (ni *NetworkInterface) NetworkName() string {
	return ni.doc.NetworkName
}

// MachineId returns the machine id of the interface.
func (ni *NetworkInterface) MachineId() string {
	return ni.doc.MachineId
}

// Remove deletes the network interface from state.
func (ni *NetworkInterface) Remove() error {
	ops := []txn.Op{{
		C:      ni.st.networkInterfaces.Name,
		Id:     ni.doc.MACAddress,
		Remove: true,
	}}
	err := ni.st.runTransaction(ops)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("network interface %q", ni.doc.MACAddress)
	}
	return err
}

func removeNetworkInterfacesOps(st *State, machineId string) []txn.Op {
	var doc struct {
		MACAddress string `bson:"_id"`
	}
	ops := []txn.Op{}
	sel := bson.D{{"machineid", machineId}}
	iter := st.networkInterfaces.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      st.networkInterfaces.Name,
			Id:     doc.MACAddress,
			Remove: true,
		})
	}
	return ops
}
