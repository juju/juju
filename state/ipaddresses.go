// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/errors"
	"github.com/juju/juju/network"
)

// AddressState represents the states an IP address can be in. They are created
// in an unknown state and then either become allocated or unavailable if
// allocation fails.
type AddressState string

const (
	// AddressStateUnknown is the initial state an IP address is created with
	AddressStateUnknown AddressState = ""

	// AddressStateAllocated means that the IP address has successfully
	// been allocated by the provider and is now in use.
	AddressStateAllocated AddressState = "allocated"

	// AddressStateUnavailable means that allocating the address with the
	// provider failed, presumably because it is already in use.
	AddressStateUnvailable AddressState = "unavailable"
)

// IPAddress represents the state of an IP address.
type IPAddress struct {
	st  *State
	doc ipaddressDoc
}

type ipaddressDoc struct {
	DocID       string `bson:"_id"`
	EnvUUID     string `bson:"env-uuid"`
	SubnetId    string `bson:",omitempty"`
	MachineId   string `bson:",omitempty"`
	InterfaceId string `bson:",omitempty"`
	Value       string
	Type        network.AddressType
	Scope       network.Scope `bson:"networkscope,omitempty"`
	State       AddressState
}

// SubnetId returns the ID of the subnet the IP address is associated with. If
// the address is not associated with a subnet this returns "".
func (i *IPAddress) SubnetId() string {
	return i.doc.SubnetId
}

// MachineId returns the ID of the machine the IP address is associated with. If
// the address is not associated with a machine this returns "".
func (i *IPAddress) MachineId() string {
	return i.doc.MachineId
}

// InterfaceId returns the ID of the network interface the IP address is
// associated with. If the address is not associated with a netowrk interface
// this returns "".
func (i *IPAddress) InterfaceId() string {
	return i.doc.InterfaceId
}

// Value returns the IP address.
func (i *IPAddress) Value() string {
	return i.doc.Value
}

// Type returns the type of the IP address. The IP address will have a type of
// IPv4, IPv6 or hostname.
func (i *IPAddress) Type() network.AddressType {
	return i.doc.Type
}

// Scope returns the scope of the IP address. If the scope is not set this
// returns "".
func (i *IPAddress) Scope() network.Scope {
	return i.doc.Scope
}

// State returns the state of an IP address.
func (i *IPAddress) State() AddressState {
	return i.doc.State
}

// Remove removes a no-longer need IP address.
func (i *IPAddress) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove IP address %v", i)

	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Remove: true,
	}}
	return i.st.runTransaction(ops)
}

// SetState sets the State of an IPAddress. Valid state transitions are Unknown
// to Allocated or Unavailable. Any other transition will fail.
func (i *IPAddress) SetState(newState AddressState) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set IP address %v to state %q", i, newState)

	validStates := []AddressState{AddressStateUnknown, newState}
	unknownOrSame := bson.D{{"state", bson.D{{"$in", validStates}}}}
	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Assert: unknownOrSame,
		Update: bson.D{{"$set", bson.D{{"state", string(newState)}}}},
	}}
	return i.st.runTransaction(ops)
}

// SetMachineId sets the ID of the machine the address is associated with
func (i *IPAddress) SetMachineId(machineId string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set IP address %v to machine ID %q", i, machineId)

	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"machineid", machineId}}}},
	}}
	return i.st.runTransaction(ops)
}

// SetInterfaceId sets the ID of the network interface the address is associated with
func (i *IPAddress) SetInterfaceId(interfaceId string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set IP address %v to interface ID %q", i, interfaceId)

	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"interfaceid", interfaceId}}}},
	}}
	return i.st.runTransaction(ops)
}
