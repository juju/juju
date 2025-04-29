// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/virtualhostname"
)

// unitHostKeyID provides the virtual host key
// lookup value for a unit based on the unit name.
func unitHostKeyID(unitName string) string {
	return "unit" + "-" + unitName + "-" + "hostkey"
}

// machineHostKeyID provides the virtual host key
// lookup value for a machine based on the machine ID.
func machineHostKeyID(machineID string) string {
	return "machine" + "-" + machineID + "-" + "hostkey"
}

// VirtualHostKey represents the state of a virtual host key.
type VirtualHostKey struct {
	doc virtualHostKeyDoc
}

type virtualHostKeyDoc struct {
	DocId   string `bson:"_id"`
	HostKey []byte `bson:"hostkey"`
}

// HostKey returns the virtual host key.
func (s *VirtualHostKey) HostKey() []byte {
	return s.doc.HostKey
}

func newVirtualHostKeyDoc(modelUUID, hostKeyID string, hostkey []byte) (virtualHostKeyDoc, error) {
	return virtualHostKeyDoc{
		DocId:   ensureModelUUID(modelUUID, hostKeyID),
		HostKey: hostkey,
	}, nil
}

func newMachineVirtualHostKeysOps(modelUUID, machineID string, hostKey []byte) ([]txn.Op, error) {
	hostKeyID := machineHostKeyID(machineID)
	doc, err := newVirtualHostKeyDoc(modelUUID, hostKeyID, hostKey)
	if err != nil {
		return nil, err
	}
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}, nil
}

func newUnitVirtualHostKeysOps(modelUUID, unitName string, hostKey []byte) ([]txn.Op, error) {
	hostKeyID := unitHostKeyID(unitName)
	doc, err := newVirtualHostKeyDoc(modelUUID, hostKeyID, hostKey)
	if err != nil {
		return nil, err
	}
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}, nil
}

func removeMachineVirtualHostKeyOps(state *State, machineID string) []txn.Op {
	machineLookup := machineHostKeyID(machineID)
	docID := state.docID(machineLookup)
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     docID,
		Remove: true,
	}}
}

func removeUnitVirtualHostKeysOps(state *State, unitName string) []txn.Op {
	unitLookup := unitHostKeyID(unitName)
	docID := state.docID(unitLookup)
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     docID,
		Remove: true,
	}}
}

// MachineVirtualHostKey returns the virtual host key for a machine.
func (st *State) MachineVirtualHostKey(machineID string) (*VirtualHostKey, error) {
	return st.virtualHostKey(machineHostKeyID(machineID))
}

// UnitVirtualHostKey returns the virtual host key for a unit.
func (st *State) UnitVirtualHostKey(unitID string) (*VirtualHostKey, error) {
	return st.virtualHostKey(unitHostKeyID(unitID))
}

func (st *State) virtualHostKey(id string) (*VirtualHostKey, error) {
	vhkeys, closer := st.db().GetCollection(virtualHostKeysC)
	defer closer()

	doc := virtualHostKeyDoc{}
	err := vhkeys.FindId(st.docID(id)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("virtual host key %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "getting virtual host key %q", id)
	}
	return &VirtualHostKey{
		doc: doc,
	}, nil
}

// AllVirtualHostKeys returns all virtual host keys.
func (st *State) AllVirtualHostKeys() ([]*VirtualHostKey, error) {
	var vhkDocs []virtualHostKeyDoc
	virtualHostKeysCollection, closer := st.db().GetCollection(virtualHostKeysC)
	defer closer()

	err := virtualHostKeysCollection.Find(nil).All(&vhkDocs)
	if err != nil {
		return nil, errors.Annotatef(err, "getting all virtual host keys")
	}
	virtualHostKeys := make([]*VirtualHostKey, len(vhkDocs))
	for i, doc := range vhkDocs {
		virtualHostKeys[i] = &VirtualHostKey{doc: doc}
	}

	return virtualHostKeys, nil
}

// HostKeyForVirtualHostname returns the private host key for the target virtual hostname.
func (st *State) HostKeyForVirtualHostname(info virtualhostname.Info) (*VirtualHostKey, error) {
	switch info.Target() {
	case virtualhostname.MachineTarget:
		machineId, _ := info.Machine()
		return st.MachineVirtualHostKey(strconv.Itoa(machineId))
	case virtualhostname.UnitTarget:
		unitName, _ := info.Unit()
		unit, err := st.Unit(unitName)
		if err != nil {
			return nil, err
		}
		if unit.modelType == ModelTypeIAAS {
			machineID, err := unit.AssignedMachineId()
			if err != nil {
				return nil, err
			}
			return st.MachineVirtualHostKey(machineID)
		} else {
			// Fetch the virtual host key for a CAAS unit.
			// This is the same key as any container
			// within this unit / K8s pod.
			return st.UnitVirtualHostKey(unitName)
		}
	case virtualhostname.ContainerTarget:
		unitName, _ := info.Unit()
		return st.UnitVirtualHostKey(unitName)
	default:
		return nil, errors.NotValidf("unsupported target: %v", info.Target())
	}
}
