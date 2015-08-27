// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

// constraintsDoc is the mongodb representation of a constraints.Value.
type constraintsDoc struct {
	EnvUUID      string `bson:"env-uuid"`
	Arch         *string
	CpuCores     *uint64
	CpuPower     *uint64
	Mem          *uint64
	RootDisk     *uint64
	InstanceType *string
	Container    *instance.ContainerType
	Tags         *[]string
	Spaces       *[]string
	// TODO(dimitern): Drop this once it's not possible to specify
	// networks= in constraints.
	Networks *[]string
}

func (doc constraintsDoc) value() constraints.Value {
	return constraints.Value{
		Arch:         doc.Arch,
		CpuCores:     doc.CpuCores,
		CpuPower:     doc.CpuPower,
		Mem:          doc.Mem,
		RootDisk:     doc.RootDisk,
		InstanceType: doc.InstanceType,
		Container:    doc.Container,
		Tags:         doc.Tags,
		Spaces:       doc.Spaces,
		Networks:     doc.Networks,
	}
}

func newConstraintsDoc(st *State, cons constraints.Value) constraintsDoc {
	return constraintsDoc{
		EnvUUID:      st.EnvironUUID(),
		Arch:         cons.Arch,
		CpuCores:     cons.CpuCores,
		CpuPower:     cons.CpuPower,
		Mem:          cons.Mem,
		RootDisk:     cons.RootDisk,
		InstanceType: cons.InstanceType,
		Container:    cons.Container,
		Tags:         cons.Tags,
		Spaces:       cons.Spaces,
		Networks:     cons.Networks,
	}
}

func createConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     st.docID(id),
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(st, cons),
	}
}

func setConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     st.docID(id),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", newConstraintsDoc(st, cons)}},
	}
}

func removeConstraintsOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     st.docID(id),
		Remove: true,
	}
}

func readConstraints(st *State, id string) (constraints.Value, error) {
	constraintsCollection, closer := st.getCollection(constraintsC)
	defer closer()

	doc := constraintsDoc{}
	if err := constraintsCollection.FindId(id).One(&doc); err == mgo.ErrNotFound {
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
