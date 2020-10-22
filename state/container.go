// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/container"
)

// machineContainers holds the machine ids of all the containers belonging to a parent machine.
// All machines have an associated container ref doc, regardless of whether they host any containers.
type machineContainers struct {
	DocID     string   `bson:"_id"`
	Id        string   `bson:"machineid"`
	ModelUUID string   `bson:"model-uuid"`
	Children  []string `bson:",omitempty"`
}

func addChildToContainerRefOp(mb modelBackend, parentId string, childId string) txn.Op {
	return txn.Op{
		C:      containerRefsC,
		Id:     mb.docID(parentId),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{"children", childId}}}},
	}
}

func insertNewContainerRefOp(mb modelBackend, machineId string, children ...string) txn.Op {
	return txn.Op{
		C:      containerRefsC,
		Id:     mb.docID(machineId),
		Assert: txn.DocMissing,
		Insert: &machineContainers{
			Id:       machineId,
			Children: children,
		},
	}
}

// removeContainerRefOps returns the txn.Op's necessary to remove a machine container record.
// These include removing the record itself and updating the host machine's children property.
func removeContainerRefOps(mb modelBackend, machineId string) []txn.Op {
	removeRefOp := txn.Op{
		C:      containerRefsC,
		Id:     mb.docID(machineId),
		Assert: txn.DocExists,
		Remove: true,
	}
	// If the machine is a container, figure out its parent host.
	parentId := container.ParentId(machineId)
	if parentId == "" {
		return []txn.Op{removeRefOp}
	}
	removeParentRefOp := txn.Op{
		C:      containerRefsC,
		Id:     mb.docID(parentId),
		Assert: txn.DocExists,
		Update: bson.D{{"$pull", bson.D{{"children", machineId}}}},
	}
	return []txn.Op{removeRefOp, removeParentRefOp}
}
