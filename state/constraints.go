// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

// constraintsDoc is the mongodb representation of a constraints.Value.
type constraintsDoc struct {
	EnvUUID      string                 `bson:"env-uuid"`
	Arch         string                 `bson:",omitempty"`
	CpuCores     uint64                 `bson:",omitempty"`
	CpuPower     uint64                 `bson:",omitempty"`
	Mem          uint64                 `bson:",omitempty"`
	RootDisk     uint64                 `bson:",omitempty"`
	InstanceType string                 `bson:",omitempty"`
	Container    instance.ContainerType `bson:",omitempty"`
	Tags         []string               `bson:",omitempty"`
	Spaces       []string               `bson:",omitempty"`
	// TODO(dimitern): Drop this once it's not possible to specify
	// networks= in constraints.
	Networks []string `bson:",omitempty"`
}

func (doc constraintsDoc) value() constraints.Value {
	var container *instance.ContainerType
	if len(doc.Container) > 0 {
		container = &doc.Container
	}
	return constraints.Value{
		Arch:         stringpIfSet(doc.Arch),
		CpuCores:     uint64pIfSet(doc.CpuCores),
		CpuPower:     uint64pIfSet(doc.CpuPower),
		Mem:          uint64pIfSet(doc.Mem),
		RootDisk:     uint64pIfSet(doc.RootDisk),
		InstanceType: stringpIfSet(doc.InstanceType),
		Container:    container,
		Tags:         stringslicepIfSet(doc.Tags),
		Spaces:       stringslicepIfSet(doc.Spaces),
		Networks:     stringslicepIfSet(doc.Networks),
	}
}

func uint64pIfSet(v uint64) *uint64 {
	if v > 0 {
		return &v
	}
	return nil
}

func stringpIfSet(v string) *string {
	if len(v) > 0 {
		return &v
	}
	return nil
}

func stringslicepIfSet(v []string) *[]string {
	if len(v) > 0 {
		return &v
	}
	return nil
}

func newConstraintsDoc(st *State, cons constraints.Value) constraintsDoc {
	doc := constraintsDoc{EnvUUID: st.EnvironUUID()}
	// Make sure we don't store in state truly empty constraints:
	//   (*[]string)(nil)
	//   (*[]string)(&v)  (where v := []string(nil))
	//   (*[]string)(&w)  (where w := []string{})
	//   (*string)(nil)
	//   (*string)(&s)    (where s := "")
	//   (*uint64)(nil)
	//   (*uint64)(&u)    (where u := 0)
	if cons.Arch != nil && len(*cons.Arch) > 0 {
		doc.Arch = *cons.Arch
	}
	if cons.CpuCores != nil && *cons.CpuCores > 0 {
		doc.CpuCores = *cons.CpuCores
	}
	if cons.CpuPower != nil && *cons.CpuPower > 0 {
		doc.CpuPower = *cons.CpuPower
	}
	if cons.Mem != nil && *cons.Mem > 0 {
		doc.Mem = *cons.Mem
	}
	if cons.RootDisk != nil && *cons.RootDisk > 0 {
		doc.RootDisk = *cons.RootDisk
	}
	if cons.InstanceType != nil && len(*cons.InstanceType) > 0 {
		doc.InstanceType = *cons.InstanceType
	}
	if cons.Container != nil && len(*cons.Container) > 0 {
		doc.Container = *cons.Container
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		doc.Tags = *cons.Tags
	}
	if cons.Spaces != nil && len(*cons.Spaces) > 0 {
		doc.Spaces = *cons.Spaces
	}
	if cons.Networks != nil && len(*cons.Networks) > 0 {
		doc.Networks = *cons.Networks
	}
	return doc
}

func createConstraintsOp(st *State, id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     st.docID(id),
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(st, cons),
	}
}

func setConstraintsOps(st *State, id string, cons constraints.Value) []txn.Op {
	// Remove any existing constraints doc before re-creating it. This
	// way we replace the doc rather than leaving lingering values
	// from (most likely incorrect) earlier versions, just because
	// they're not set on cons.
	return []txn.Op{
		removeConstraintsOp(st, id),
		createConstraintsOp(st, id, cons),
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
	ops := setConstraintsOps(st, id, cons)
	if err := st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set constraints: %v", err)
	}
	return nil
}
