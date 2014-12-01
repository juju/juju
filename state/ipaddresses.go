// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/network"
	"gopkg.in/mgo.v2/txn"
)

type AddressState string

const (
	AddressStateUnknown    AddressState = ""
	AddressStateAllocated  AddressState = "allocated"
	AddressStateUnvailable AddressState = "unavailable"
)

// IPAddressInfo describes a single IP address.
type IPAddressInfo struct {
	State       AddressState
	SubnetId    string
	MachineId   string
	InterfaceId string
	Value       string
	Type        network.AddressType
	Scope       network.Scope
}

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
	State       AddressState  `bson:",omitempty"`
}

func (i *IPAddress) SubnetId() string {
	return i.doc.SubnetId
}

func (i *IPAddress) MachineId() string {
	return i.doc.MachineId
}

func (i *IPAddress) InterfaceId() string {
	return i.doc.InterfaceId
}

func (i *IPAddress) Value() string {
	return i.doc.Value
}

func (i *IPAddress) Type() network.AddressType {
	return i.doc.Type
}

func (i *IPAddress) Scope() network.Scope {
	return i.doc.Scope
}

// Remove removes a dead subnet. If the subnet is not dead it returns an error.
func (i *IPAddress) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove IP address %v", i)
	ops := []txn.Op{{
		C:      ipaddressesC,
		Id:     i.doc.DocID,
		Remove: true,
	}}
	return i.st.runTransaction(ops)
}
