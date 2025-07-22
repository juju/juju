// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
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

// Constraints represents the state of a constraints with an ID.
type Constraints struct {
	doc constraintsDoc
}

// ID returns the Mongo document ID for the constraints instance.
func (c *Constraints) ID() string {
	return c.doc.DocID
}

// ConstraintsBySpaceName returns all Constraints that include a positive
// or negative space constraint for the input space name.
func (st *State) ConstraintsBySpaceName(spaceName string) ([]*Constraints, error) {
	cons := make([]*Constraints, 0)
	return cons, nil
}
