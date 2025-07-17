// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

// constraintsDoc is the Mongo DB representation of a constraints.Value.
type constraintsDoc struct {
	DocID            string `bson:"_id,omitempty"`
	ModelUUID        string `bson:"model-uuid"`
	Arch             *string
	CpuCores         *uint64
	CpuPower         *uint64
	Mem              *uint64
	RootDisk         *uint64
	RootDiskSource   *string
	InstanceRole     *string
	InstanceType     *string
	Container        *instance.ContainerType
	Tags             *[]string
	Spaces           *[]string
	VirtType         *string
	Zones            *[]string
	AllocatePublicIP *bool
	ImageID          *string
}

func (doc constraintsDoc) value() constraints.Value {
	result := constraints.Value{
		Arch:             doc.Arch,
		CpuCores:         doc.CpuCores,
		CpuPower:         doc.CpuPower,
		Mem:              doc.Mem,
		RootDisk:         doc.RootDisk,
		RootDiskSource:   doc.RootDiskSource,
		InstanceRole:     doc.InstanceRole,
		InstanceType:     doc.InstanceType,
		Container:        doc.Container,
		Tags:             doc.Tags,
		Spaces:           doc.Spaces,
		VirtType:         doc.VirtType,
		Zones:            doc.Zones,
		AllocatePublicIP: doc.AllocatePublicIP,
		ImageID:          doc.ImageID,
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

// Value returns the constraints.Value represented by the Constraints'
// Mongo document.
func (c *Constraints) Value() constraints.Value {
	return c.doc.value()
}

// ChangeSpaceNameOps returns the transaction operations required to rename
// a space used in these constraints.
func (c *Constraints) ChangeSpaceNameOps(from, to string) []txn.Op {
	return nil
}

// AllConstraints returns all constraints in the collection.
func (st *State) AllConstraints() ([]*Constraints, error) {
	cons := make([]*Constraints, 0)
	return cons, nil
}

// ConstraintsBySpaceName returns all Constraints that include a positive
// or negative space constraint for the input space name.
func (st *State) ConstraintsBySpaceName(spaceName string) ([]*Constraints, error) {
	cons := make([]*Constraints, 0)
	return cons, nil
}
