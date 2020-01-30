// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

// constraintsDoc is the mongodb representation of a constraints.Value.
type constraintsDoc struct {
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

func newConstraintsDoc(cons constraints.Value) constraintsDoc {
	result := constraintsDoc{
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

func createConstraintsOp(id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: newConstraintsDoc(cons),
	}
}

func setConstraintsOp(id string, cons constraints.Value) txn.Op {
	return txn.Op{
		C:      constraintsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", newConstraintsDoc(cons)}},
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

// ConstraintsTagForSpaceName returns the tags for the given space.
func (st *State) ConstraintsTagForSpaceName(name string) ([]names.Tag, error) {
	docs, err := st.constraintsBySpaceName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := make([]names.Tag, len(docs))
	for i, doc := range docs {
		tag := st.ParseLocalIDToTags(doc.DocID)
		if tag == nil {
			logger.Debugf("Could not parse id: %q", doc.DocID)
			return nil, errors.Errorf("Could not parse id: %q", doc.DocID)
		}
		tags[i] = tag
	}
	return tags, nil
}

func (st *State) constraintsBySpaceName(name string) ([]constraintsWithID, error) {
	constraintsCollection, closer := st.db().GetCollection(constraintsC)
	defer closer()
	var docs []constraintsWithID
	negatedSpace := fmt.Sprintf("^%v", name)
	query := bson.D{{"$or", []bson.D{
		{{"spaces", name}},
		{{"spaces", negatedSpace}},
	}}}
	err := constraintsCollection.Find(query).All(&docs)
	return docs, err
}

// ConstraintsOpsForSpaceNameChange returns all the database transaction operation required
// to transform a constraints spaces from `a` to `b`
func (st *State) ConstraintsOpsForSpaceNameChange(from, to string) ([]txn.Op, error) {
	docs, err := st.constraintsBySpaceName(from)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cons := getConstraintsChanges(docs, from, to)

	ops := make([]txn.Op, len(docs))
	i := 0
	for docID, constraint := range cons {
		ops[i] = setConstraintsOp(docID, constraint)
		i++
	}
	return ops, nil
}

func getConstraintsChanges(cons []constraintsWithID, from, to string) map[string]constraints.Value {
	negatedFrom := fmt.Sprintf("^%v", from)
	negatedTo := fmt.Sprintf("^%v", to)

	values := make(map[string]constraints.Value, len(cons))
	for _, con := range cons {
		values[con.DocID] = con.Nested.value()
	}
	for _, constraint := range values {
		spaces := constraint.Spaces
		if spaces == nil {
			continue
		}
		for i, space := range *spaces {
			if space == from {
				(*spaces)[i] = to
				break
			}
			if space == negatedFrom {
				(*spaces)[i] = negatedTo
				break
			}
		}
	}
	return values
}
