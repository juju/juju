// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

// constraintsDoc is the mongodb representation of a constraints.Value.
type constraintsDoc struct {
	Arch      *string
	CpuCores  *uint64
	CpuPower  *uint64
	Mem       *uint64
	RootDisk  *uint64
	Container *instance.ContainerType
	Tags      *[]string ",omitempty"
}

func (doc constraintsDoc) value() constraints.Value {
	return constraints.Value{
		Arch:      doc.Arch,
		CpuCores:  doc.CpuCores,
		CpuPower:  doc.CpuPower,
		Mem:       doc.Mem,
		RootDisk:  doc.RootDisk,
		Container: doc.Container,
		Tags:      doc.Tags,
	}
}

func newConstraintsDoc(cons constraints.Value) constraintsDoc {
	return constraintsDoc{
		Arch:      cons.Arch,
		CpuCores:  cons.CpuCores,
		CpuPower:  cons.CpuPower,
		Mem:       cons.Mem,
		RootDisk:  cons.RootDisk,
		Container: cons.Container,
		Tags:      cons.Tags,
	}
}

func createConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(cons),
	}
}

func setConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: D{{"$set", newConstraintsDoc(cons)}},
	}
}

func removeConstraintsOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.constraints.Name,
		Id:     id,
		Remove: true,
	}
}

func readConstraints(st *State, id string) (constraints.Value, error) {
	doc := constraintsDoc{}
	if err := st.constraints.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return constraints.Value{}, errors.NotFoundf("constraints")
	} else if err != nil {
		return constraints.Value{}, err
	}
	return doc.value(), nil
}

func writeConstraints(st *State, id string, cons constraints.Value) error {
	ops := []txn.Op{setConstraintsOp(st, id, cons)}
	if err := st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set constraints: %v", err)
	}
	return nil
}
