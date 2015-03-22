// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
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

// GoString implements fmt.GoStringer.
func (i *IPAddress) GoString() string {
	return i.String()
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
	return network.NewScopedAddress(i.doc.Value, i.Scope())
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
	defer errors.DeferredAnnotatef(&err, "cannot set address %q to dead", i)

	if i.doc.Life == Dead {
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := i.Refresh(); err != nil {
				// Address is either gone or
				// another error occurred.
				return nil, err
			}
			if i.Life() == Dead {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Errorf("unexpected life value: %s", i.Life().String())
		}
		return []txn.Op{{
			C:      ipaddressesC,
			Id:     i.doc.DocID,
			Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
			Assert: isAliveDoc,
		}}, nil
	}

	err = i.st.run(buildTxn)
	if err != nil {
		return err
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

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := i.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
			if i.Life() != Dead {
				return nil, errors.New("address is not dead")
			}
		}
		return []txn.Op{{
			C:      ipaddressesC,
			Id:     i.doc.DocID,
			Assert: isDeadDoc,
			Remove: true,
		}}, nil
	}

	return i.st.run(buildTxn)
}

// SetState sets the State of an IPAddress. Valid state transitions
// are Unknown to Allocated or Unavailable, as well as setting the
// same state more than once. Any other transition will result in
// returning an error satisfying errors.IsNotValid().
func (i *IPAddress) SetState(newState AddressState) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set IP address %q to state %q", i, newState)

	validStates := []AddressState{AddressStateUnknown, newState}
	unknownOrSame := bson.DocElem{"state", bson.D{{"$in", validStates}}}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := i.Refresh(); errors.IsNotFound(err) {
				return nil, err
			} else if i.Life() == Dead {
				return nil, errors.New("address is dead")
			} else if i.State() != AddressStateUnknown {
				return nil, errors.NotValidf("transition from %q", i.doc.State)
			} else if err != nil {
				return nil, err
			}

		}
		return []txn.Op{{
			C:      ipaddressesC,
			Id:     i.doc.DocID,
			Assert: append(isAliveDoc, unknownOrSame),
			Update: bson.D{{"$set", bson.D{{"state", string(newState)}}}},
		}}, nil
	}

	err = i.st.run(buildTxn)
	if err != nil {
		return err
	}

	i.doc.State = newState
	return nil
}

// AllocateTo sets the machine ID and interface ID of the IP address.
// It will fail if the state is not AddressStateUnknown. On success,
// the address state will also change to AddressStateAllocated.
func (i *IPAddress) AllocateTo(machineId, interfaceId string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot allocate IP address %q to machine %q, interface %q", i, machineId, interfaceId)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := i.Refresh(); errors.IsNotFound(err) {
				return nil, err
			} else if i.Life() == Dead {
				return nil, errors.New("address is dead")
			} else if i.State() != AddressStateUnknown {
				return nil, errors.Errorf("already allocated or unavailable")
			} else if err != nil {
				return nil, err
			}

		}
		return []txn.Op{{
			C:      ipaddressesC,
			Id:     i.doc.DocID,
			Assert: append(isAliveDoc, bson.DocElem{"state", AddressStateUnknown}),
			Update: bson.D{{"$set", bson.D{
				{"machineid", machineId},
				{"interfaceid", interfaceId},
				{"state", string(AddressStateAllocated)},
			}}},
		}}, nil
	}

	err = i.st.run(buildTxn)
	if err != nil {
		return err
	}
	i.doc.MachineId = machineId
	i.doc.InterfaceId = interfaceId
	i.doc.State = AddressStateAllocated
	return nil
}

// Refresh refreshes the contents of the IPAddress from the underlying
// state. It an error that satisfies errors.IsNotFound if the Subnet has
// been removed.
func (i *IPAddress) Refresh() error {
	addresses, closer := i.st.getCollection(ipaddressesC)
	defer closer()

	err := addresses.FindId(i.doc.DocID).One(&i.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("IP address %q", i)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh IP address %q", i)
	}
	return nil
}
