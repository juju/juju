// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "labix.org/v2/mgo/txn"

type ContainerType string

const (
	LXC = ContainerType("lxc")
	KVM = ContainerType("kvm")
)

// SupportedContainerTypes is used to validate add-machine arguments.
var SupportedContainerTypes []ContainerType = []ContainerType{
	LXC,
}

// updateChildrenRefCountOp returns the operations needed to update the given
// NumChildren attribute on the machine with the specified id by the specified increment amount.
func updateChildrenRefCountOp(st *State, machineId string, countInc int) txn.Op {
	return txn.Op{
		C:      st.machines.Name,
		Id:     machineId,
		Assert: txn.DocExists,
		Update: D{{"$inc", D{{"numchildren", countInc}}}},
	}
}
