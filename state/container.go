// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
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
