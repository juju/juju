// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/tools"
)

// MachineRef is a reference to a machine, without being a full machine.
// This exists to allow us to use state functions without requiring a
// state.Machine, without having to require a real machine.
type MachineRef interface {
	Id() string
	Life() Life
	ContainerType() instance.ContainerType
}

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in core/multiwatcher.
type unitDoc struct {
	DocID                  string `bson:"_id"`
	Name                   string `bson:"name"`
	ModelUUID              string `bson:"model-uuid"`
	Base                   Base   `bson:"base"`
	Application            string
	CharmURL               *string
	Principal              string
	Subordinates           []string
	StorageAttachmentCount int `bson:"storageattachmentcount"`
	MachineId              string
	Tools                  *tools.Tools `bson:",omitempty"`
	Life                   Life
	PasswordHash           string
}

// Unit represents the state of an application unit.
type Unit struct {
	st  *State
	doc unitDoc
}

func newUnit(st *State, modelType ModelType, udoc *unitDoc) *Unit {
	unit := &Unit{
		st:  st,
		doc: *udoc,
	}
	return unit
}

// Tag returns a name identifying the unit.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (u *Unit) Tag() names.Tag {
	return names.NewUnitTag("no/0")
}

// ActionSpecsByName is a map of action names to their respective ActionSpec.
type ActionSpecsByName map[string]charm.ActionSpec
