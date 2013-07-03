// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo/txn"
	"strings"
)

// machineContainers holds the machine ids of all the containers belonging to a parent machine.
// All machines have an associated container ref doc, regardless of whether they host any containers.
type machineContainers struct {
	Id       string   `bson:"_id"`
	Children []string `bson:",omitempty"`
}

// createContainerRefOp the txn.Op's that update a parent machine's container references to add
// a new container with the specified container id. If no container id is specified, an empty
// machineContainers doc is created.
func createContainerRefOp(st *State, params containerRefParams) []txn.Op {
	// See if we are creating a parent machine to subsequently host a new container. In this case,
	// the container ref doc will be created later once the container id is known.
	if !params.hostOnly && params.containerId == "" {
		return []txn.Op{}
	}
	if params.hostOnly {
		// Create an empty containers reference for a new host machine which will have no containers.
		return []txn.Op{
			{
				C:      st.containerRefs.Name,
				Id:     params.hostId,
				Assert: txn.DocMissing,
				Insert: &machineContainers{
					Id: params.hostId,
				},
			},
		}
	}
	var ops []txn.Op
	if params.newHost {
		// If the host machine doesn't exist yet, create a new containers record.
		mc := machineContainers{
			Id:       params.hostId,
			Children: []string{params.containerId},
		}
		ops = []txn.Op{
			{
				C:      st.containerRefs.Name,
				Id:     mc.Id,
				Assert: txn.DocMissing,
				Insert: &mc,
			},
		}
	} else {
		// The host machine exists so update it's containers record.
		ops = []txn.Op{
			{
				C:      st.containerRefs.Name,
				Id:     params.hostId,
				Assert: txn.DocExists,
				Update: D{{"$addToSet", D{{"children", params.containerId}}}},
			},
		}
	}
	// Create a containers reference document for the container itself.
	mc := machineContainers{
		Id: params.containerId,
	}
	ops = append(ops, txn.Op{
		C:      st.containerRefs.Name,
		Id:     mc.Id,
		Assert: txn.DocMissing,
		Insert: &mc,
	})
	return ops
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

// removeContainerRefOps returns the txn.Op's necessary to remove a machine container record.
// These include removing the record itself and updating the host machine's children property.
func removeContainerRefOps(st *State, machineId string) []txn.Op {
	removeRefOp := txn.Op{
		C:      st.containerRefs.Name,
		Id:     machineId,
		Assert: txn.DocExists,
		Remove: true,
	}
	// If the machine is a container, figure out it's parent host.
	parentId := ParentId(machineId)
	if parentId == "" {
		return []txn.Op{removeRefOp}
	}
	removeParentRefOp := txn.Op{
		C:      st.containerRefs.Name,
		Id:     parentId,
		Assert: txn.DocExists,
		Update: D{{"$pull", D{{"children", machineId}}}},
	}
	return []txn.Op{removeRefOp, removeParentRefOp}
}
