// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// addIPAddress implements the State method to add an IP address.
func addIPAddress(st *State, addr network.Address, subnetid string) (ipaddress *IPAddress, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add IP address %q", addr)

	// This checks for a missing value as well as invalid values
	ip := net.ParseIP(addr.Value)
	if ip == nil {
		return nil, errors.NotValidf("address")
	}

	// Generate the UUID for the new IP address.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	addressID := st.docID(addr.Value)
	ipDoc := ipaddressDoc{
		DocID:    addressID,
		EnvUUID:  st.EnvironUUID(),
		UUID:     uuid.String(),
		Life:     Alive,
		State:    AddressStateUnknown,
		SubnetId: subnetid,
		Value:    addr.Value,
		Type:     string(addr.Type),
		Scope:    string(addr.Scope),
	}

	ipaddress = &IPAddress{doc: ipDoc, st: st}
	ops := []txn.Op{
		assertEnvAliveOp(st.EnvironUUID()),
		{
			C:      ipaddressesC,
			Id:     addressID,
			Assert: txn.DocMissing,
			Insert: ipDoc,
		},
	}

	err = st.runTransaction(ops)
	switch err {
	case txn.ErrAborted:
		if err := checkEnvLife(st); err != nil {
			return nil, errors.Trace(err)
		}
		if _, err = st.IPAddress(addr.Value); err == nil {
			return nil, errors.AlreadyExistsf("address")
		}
	case nil:
		return ipaddress, nil
	}
	return nil, errors.Trace(err)
}

// ipAddress implements the State method to return an existing IP
// address by its value.
func ipAddress(st *State, value string) (*IPAddress, error) {
	addresses, closer := st.getCollection(ipaddressesC)
	defer closer()

	doc := &ipaddressDoc{}
	err := addresses.FindId(value).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("IP address %q", value)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get IP address %q", value)
	}
	return &IPAddress{st, *doc}, nil
}

// ipAddressByTag implements the State method to return an existing IP
// address by its tag.
func ipAddressByTag(st *State, tag names.IPAddressTag) (*IPAddress, error) {
	addresses, closer := st.getCollection(ipaddressesC)
	defer closer()

	doc := &ipaddressDoc{}
	err := addresses.Find(bson.D{{"uuid", tag.Id()}}).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("IP address %q", tag)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get IP address %q", tag)
	}
	return &IPAddress{st, *doc}, nil
}

// fetchIPAddresses is a helper function for finding IP addresses
func fetchIPAddresses(st *State, query bson.D) ([]*IPAddress, error) {
	addresses, closer := st.getCollection(ipaddressesC)
	result := []*IPAddress{}
	defer closer()
	doc := ipaddressDoc{}
	iter := addresses.Find(query).Iter()
	for iter.Next(&doc) {
		addr := &IPAddress{
			st:  st,
			doc: doc,
		}
		result = append(result, addr)
	}
	if err := iter.Close(); err != nil {
		return result, err
	}
	return result, nil
}

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
	UUID        string       `bson:"uuid"`
	Life        Life         `bson:"life"`
	SubnetId    string       `bson:"subnetid,omitempty"`
	MachineId   string       `bson:"machineid,omitempty"`
	MACAddress  string       `bson:"macaddress,omitempty"`
	InstanceId  string       `bson:"instanceid,omitempty"`
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

// Id returns the ID of the IP address.
func (i *IPAddress) Id() string {
	return i.doc.DocID
}

// UUID returns the globally unique ID of the IP address.
func (i *IPAddress) UUID() (utils.UUID, error) {
	return utils.UUIDFromString(i.doc.UUID)
}

// Tag returns the tag of the IP address.
func (i *IPAddress) Tag() names.Tag {
	return names.NewIPAddressTag(i.doc.UUID)
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

// InstanceId returns the provider ID of the instance the IP address is
// associated with. For a container this will be the ID of the host. If
// the address is not associated with an instance this returns "" (the same as
// instance.UnknownId).
func (i *IPAddress) InstanceId() instance.Id {
	return instance.Id(i.doc.InstanceId)
}

// MACAddress returns the MAC address of the container NIC the IP address is
// associated with.
func (i *IPAddress) MACAddress() string {
	return i.doc.MACAddress
}

// InterfaceId returns the ID of the network interface the IP address is
// associated with. If the address is not associated with a network interface
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

// GoString implements fmt.GoStringer.
func (i *IPAddress) GoString() string {
	return i.String()
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
		op := ensureIPAddressDeadOp(i)
		op.Assert = isAliveDoc
		return []txn.Op{op}, nil
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

// AllocateTo sets the machine ID, MAC address and interface ID of the IP address.
// It will fail if the state is not AddressStateUnknown. On success,
// the address state will also change to AddressStateAllocated.
func (i *IPAddress) AllocateTo(machineId, interfaceId, macAddress string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot allocate IP address %q to machine %q, interface %q", i, machineId, interfaceId)

	var instId instance.Id
	machine, err := i.st.Machine(machineId)
	if err != nil {
		return errors.Annotatef(err, "cannot get allocated machine %q", machineId)
	} else {
		instId, err = machine.InstanceId()

		if errors.IsNotProvisioned(err) {
			// The machine is not yet provisioned. The instance ID will be
			// set on provisioning.
			instId = instance.UnknownId
		} else if err != nil {
			return errors.Annotatef(err, "cannot get machine %q instance ID", machineId)
		}
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := checkEnvLife(i.st); err != nil {
				return nil, errors.Trace(err)
			}
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
		return []txn.Op{
			assertEnvAliveOp(i.st.EnvironUUID()),
			{
				C:      ipaddressesC,
				Id:     i.doc.DocID,
				Assert: append(isAliveDoc, bson.DocElem{"state", AddressStateUnknown}),
				Update: bson.D{{"$set", bson.D{
					{"machineid", machineId},
					{"interfaceid", interfaceId},
					{"instanceid", instId},
					{"macaddress", macAddress},
					{"state", string(AddressStateAllocated)},
				}}},
			}}, nil
	}

	err = i.st.run(buildTxn)
	if err != nil {
		return err
	}
	i.doc.MachineId = machineId
	i.doc.MACAddress = macAddress
	i.doc.InterfaceId = interfaceId
	i.doc.State = AddressStateAllocated
	i.doc.InstanceId = string(instId)
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

func ensureIPAddressDeadOp(addr *IPAddress) txn.Op {
	op := txn.Op{
		C:      ipaddressesC,
		Id:     addr.Id(),
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}
	return op
}
