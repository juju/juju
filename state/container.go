// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/txn"
	"strings"
)

type ContainerType string

const (
	LXC = ContainerType("lxc")
	KVM = ContainerType("kvm")
)

// SupportedContainerTypes is used to validate add-machine arguments.
var SupportedContainerTypes []ContainerType = []ContainerType{
	LXC,
}

// machineContainers holds the machine ids of all the containers belonging to a parent machine.
type machineContainers struct {
	Id       string   `bson:"_id"`
	Children []string `bson:",omitempty"`
}

// createContainerRefOp returns a txn.Op that updates a parent machine's container references to add
// a new container with the specified container id. If not container id is specified, an empty
// machineContainers doc is created.
func createContainerRefOp(st *State, parentId, containerId string) txn.Op {
	var op txn.Op
	if containerId != "" {
		op = txn.Op{
			C:      st.containerRefs.Name,
			Id:     parentId,
			Update: D{{"$addToSet", D{{"children", containerId}}}},
		}
	} else {
		mc := machineContainers{
			Id: parentId,
		}
		op = txn.Op{
			C:      st.containerRefs.Name,
			Id:     parentId,
			Assert: txn.DocMissing,
			Insert: &mc,
		}
	}
	return op
}

// removeContainerRefOps returns the txn.Ops necessary to remove a machine container record.
// These include removing the record itself and updating the host machine's children property.
func removeContainerRefOps(st *State, machineId string) []txn.Op {
	removeRefOp := txn.Op{
		C:      st.containerRefs.Name,
		Id:     machineId,
		Assert: txn.DocExists,
		Remove: true,
	}
	// If the machine is a container, figure out it's parent host.
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return []txn.Op{removeRefOp}
	}
	parentId := strings.Join(idParts[:len(idParts)-2], "/")
	removeParentRefOp := txn.Op{
		C:      st.containerRefs.Name,
		Id:     parentId,
		Assert: txn.DocExists,
		Update: D{{"$pull", D{{"children", machineId}}}},
	}
	return []txn.Op{removeRefOp, removeParentRefOp}
}
