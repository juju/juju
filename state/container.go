// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/instance"
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
	parentId := ParentId(machineId)
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

// ParentId returns the id of the host machine if machineId a container id, or ""
// if machineId is not for a container.
func ParentId(machineId string) string {
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return ""
	}
	return strings.Join(idParts[:len(idParts)-2], "/")
}

// ContainerTypeFromId returns the container type if machineId is a container id, or ""
// if machineId is not for a container.
func ContainerTypeFromId(machineId string) instance.ContainerType {
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return instance.ContainerType("")
	}
	return instance.ContainerType(idParts[len(idParts)-2])
}

// NestingLevel returns how many levels of nesting exist for a machine id.
func NestingLevel(machineId string) int {
	idParts := strings.Split(machineId, "/")
	return (len(idParts) - 1) / 2
}

// TopParentId returns the id of the top level host machine for a container id.
func TopParentId(machineId string) string {
	idParts := strings.Split(machineId, "/")
	return idParts[0]
}
