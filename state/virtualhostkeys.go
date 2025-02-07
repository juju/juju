// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/virtualhostkeys"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"
)

// VirtualHostKey represents the state of a virtual host key.
type VirtualHostKey struct {
	st  *State
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

func newVirtualHostKeyDoc(st *State, hostKeyID string, hostkey []byte) (virtualHostKeyDoc, error) {
	return virtualHostKeyDoc{
		DocId:   st.docID(hostKeyID),
		HostKey: hostkey,
	}, nil
}

func newMachineVirtualHostKeysOps(st *State, machineID string, hostKey []byte) ([]txn.Op, error) {
	hostKeyID := virtualhostkeys.MachineHostKeyID(machineID)
	doc, err := newVirtualHostKeyDoc(st, hostKeyID.ID, hostKey)
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

func newUnitVirtualHostKeysOps(st *State, unitName string, hostKey []byte) ([]txn.Op, error) {
	hostKeyID := virtualhostkeys.UnitHostKeyID(unitName)
	doc, err := newVirtualHostKeyDoc(st, hostKeyID.ID, hostKey)
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
	machineLookup := virtualhostkeys.MachineHostKeyID(machineID)
	docID := state.docID(machineLookup.ID)
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     docID,
		Remove: true,
	}}
}

func removeUnitVirtualHostKeysOps(state *State, unitName string) []txn.Op {
	unitLookup := virtualhostkeys.UnitHostKeyID(unitName)
	docID := state.docID(unitLookup.ID)
	return []txn.Op{{
		C:      virtualHostKeysC,
		Id:     docID,
		Remove: true,
	}}
}

func (st *State) MachineVirtualHostKey(lookup virtualhostkeys.MachineLookup) (*VirtualHostKey, error) {
	return st.virtualHostKey(lookup.ID)
}

func (st *State) UnitVirtualHostKey(lookup virtualhostkeys.UnitLookup) (*VirtualHostKey, error) {
	return st.virtualHostKey(lookup.ID)
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
		st:  st,
		doc: doc,
	}, nil
}
