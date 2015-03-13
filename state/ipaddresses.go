// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// AddressState represents the states an IP address can be in. They are created
// in an unknown state and then either become allocated or unavailable if
// allocation fails.
type AddressState string

const (
	// AddressStateUnknown is the initial state an IP address is
	// created with.
	AddressStateUnknown AddressState = ""

	// AddressStateAllocated means that the IP address has
	// successfully been allocated by the provider and is now in use
	// by an interface on a machine.
	AddressStateAllocated AddressState = "allocated"

	// AddressStateUnavailable means that allocating the address with
	// the provider failed. We shouldn't use this address, nor should
	// we attempt to allocate it again in the future.
	AddressStateUnavailable AddressState = "unavailable"
)

// String implements fmt.Stringer.
func (s AddressState) String() string {
	if s == AddressStateUnknown {
		return "<unknown>"
	}
	return string(s)
}

// IPAddress represents the state of an IP address.
type IPAddress struct {
	st  *State
	doc ipaddressDoc
}

type ipaddressDoc struct {
	DocID       string       `bson:"_id"`
	EnvUUID     string       `bson:"env-uuid"`
	Life        Life         `bson:"life"`
	SubnetId    string       `bson:"subnetid,omitempty"`
	MachineId   string       `bson:"machineid,omitempty"`
	InterfaceId string       `bson:"interfaceid,omitempty"`
	Value       string       `bson:"value"`
	Type        string       `bson:"type"`
	Scope       string       `bson:"networkscope,omitempty"`
	State       AddressState `bson:"state"`
}

// Life returns whether the IP address is Alive, Dying or Dead.
func (i *IPAddress) Life() Life {
	return i.doc.Life
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

// Address returns the network.Address represent the IP address
func (i *IPAddress) Address() network.Address {
	return network.NewAddress(i.doc.Value, i.Scope())
}

// Type returns the type of the IP address. The IP address will have a type of
// IPv4, IPv6 or hostname.
func (i *IPAddress) Type() network.AddressType {
	return network.AddressType(i.doc.Type)
}

// Scope returns the scope of the IP address. If the scope is not set this
// returns "".
func (i *IPAddress) Scope() network.Scope {
	return network.Scope(i.doc.Scope)
}

// State returns the state of an IP address.
func (i *IPAddress) State() AddressState {
	return i.doc.State
}

// String implements fmt.Stringer.
func (i *IPAddress) String() string {
	return i.Address().String()
}

// EnsureDead sets the Life of the IP address to Dead, if it's Alive. It
// does nothing otherwise.
func (i *IPAddress) EnsureDead() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set subnet %q to dead", i)

	if i.doc.Life == Dead {
		return nil
	}

	ops := []txn.Op{{
		C:      subnetsC,
		Id:     i.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		Assert: isAliveDoc,
	}}
	if err = i.st.runTransaction(ops); err != nil {
		// Ignore ErrAborted if it happens, otherwise return err.
		return onAbort(err, nil)
	}
	i.doc.Life = Dead
	return nil
}

// Remove removes an existing IP address. Trying to remove a missing
// address is not an error.
func (i *IPAddress) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove IP address %q", i)

	if i.doc.Life != Dead {
		return errors.New("IP address is not dead")
	}

	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Remove: true,
	}}
	return i.st.runTransaction(ops)
}

// SetState sets the State of an IPAddress. Valid state transitions
// are Unknown to Allocated or Unavailable, as well as setting the
// same state more than once. Any other transition will result in
// returning an error satisfying errors.IsNotValid().
func (i *IPAddress) SetState(newState AddressState) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set IP address %q to state %q", i, newState)

	validStates := []AddressState{AddressStateUnknown, newState}
	unknownOrSame := bson.D{{"state", bson.D{{"$in", validStates}}}}
	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Assert: unknownOrSame,
		Update: bson.D{{"$set", bson.D{{"state", string(newState)}}}},
	}}
	if err = i.st.runTransaction(ops); err != nil {
		return onAbort(
			err,
			errors.NotValidf("transition from %q", i.doc.State),
		)
	}
	i.doc.State = newState
	return nil
}

// AllocateTo sets the machine ID and interface ID of the IP address.
// It will fail if the state is not AddressStateUnknown. On success,
// the address state will also change to AddressStateAllocated.
func (i *IPAddress) AllocateTo(machineId, interfaceId string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot allocate IP address %q to machine %q, interface %q", i, machineId, interfaceId)

	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Assert: bson.D{{"state", AddressStateUnknown}},
		Update: bson.D{{"$set", bson.D{
			{"machineid", machineId},
			{"interfaceid", interfaceId},
			{"state", string(AddressStateAllocated)},
		}}},
	}}

	if err = i.st.runTransaction(ops); err != nil {
		return onAbort(
			err,
			errors.Errorf("already allocated or unavailable"),
		)
	}
	i.doc.MachineId = machineId
	i.doc.InterfaceId = interfaceId
	i.doc.State = AddressStateAllocated
	return nil
}
