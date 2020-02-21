// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

// constraintsDoc is the Mongo DB representation of a constraints.Value.
type constraintsDoc struct {
	DocID          string `bson:"_id,omitempty"`
	ModelUUID      string `bson:"model-uuid"`
	Arch           *string
	CpuCores       *uint64
	CpuPower       *uint64
	Mem            *uint64
	RootDisk       *uint64
	RootDiskSource *string
	InstanceType   *string
	Container      *instance.ContainerType
	Tags           *[]string
	Spaces         *[]string
	VirtType       *string
	Zones          *[]string
}

func newConstraintsDoc(cons constraints.Value, id string) constraintsDoc {
	result := constraintsDoc{
		DocID:          id,
		Arch:           cons.Arch,
		CpuCores:       cons.CpuCores,
		CpuPower:       cons.CpuPower,
		Mem:            cons.Mem,
		RootDisk:       cons.RootDisk,
		RootDiskSource: cons.RootDiskSource,
		InstanceType:   cons.InstanceType,
		Container:      cons.Container,
		Tags:           cons.Tags,
		Spaces:         cons.Spaces,
		VirtType:       cons.VirtType,
		Zones:          cons.Zones,
	}
	return result
}

func (doc constraintsDoc) value() constraints.Value {
	result := constraints.Value{
		Arch:           doc.Arch,
		CpuCores:       doc.CpuCores,
		CpuPower:       doc.CpuPower,
		Mem:            doc.Mem,
		RootDisk:       doc.RootDisk,
		RootDiskSource: doc.RootDiskSource,
		InstanceType:   doc.InstanceType,
		Container:      doc.Container,
		Tags:           doc.Tags,
		Spaces:         doc.Spaces,
		VirtType:       doc.VirtType,
		Zones:          doc.Zones,
	}
	return result
}

// Constraints represents the state of a constraints with an ID.
type Constraints struct {
	doc constraintsDoc
}

// ID returns the Mongo document ID for the constraints instance.
func (c *Constraints) ID() string {
	return c.doc.DocID
}

// ChangeSpaceNameOps returns the transaction operations required to rename
// a space used in these constraints.
func (c *Constraints) ChangeSpaceNameOps(from, to string) []txn.Op {
	val := c.doc.value()
	spaces := val.Spaces
	if spaces == nil {
		return nil
	}

	for i, space := range *spaces {
		if space == from {
			(*spaces)[i] = to
			break
		}
		if space == "^"+from {
			(*spaces)[i] = "^" + to
		}
	}

	return []txn.Op{setConstraintsOp(c.ID(), val)}
}

// ConstraintsBySpaceName returns all Constraints that include a positive
// or negative space constraint for the input space name.
func (st *State) ConstraintsBySpaceName(spaceName string) ([]*Constraints, error) {
	constraintsCollection, closer := st.db().GetCollection(constraintsC)
	defer closer()

	var docs []constraintsDoc
	query := bson.D{{"$or", []bson.D{
		{{"spaces", spaceName}},
		{{"spaces", "^" + spaceName}},
	}}}
	err := constraintsCollection.Find(query).All(&docs)

	cons := make([]*Constraints, len(docs))
	for i, doc := range docs {
		cons[i] = &Constraints{doc: doc}
	}
	return cons, err
}

func createConstraintsOp(id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(cons, id),
	}
}

func setConstraintsOp(id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", newConstraintsDoc(cons, id)}},
	}
}

func removeConstraintsOp(id string) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     id,
		Remove: true,
	}
}

func readConstraints(mb modelBackend, id string) (constraints.Value, error) {
	constraintsCollection, closer := mb.db().GetCollection(constraintsC)
	defer closer()

	doc := constraintsDoc{}
	if err := constraintsCollection.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return constraints.Value{}, errors.NotFoundf("constraints")
	} else if err != nil {
		return constraints.Value{}, err
	}
	return doc.value(), nil
}

func writeConstraints(mb modelBackend, id string, cons constraints.Value) error {
	ops := []txn.Op{setConstraintsOp(id, cons)}
	if err := mb.db().RunTransaction(ops); err != nil {
		return fmt.Errorf("cannot set constraints: %v", err)
	}
	return nil
}
