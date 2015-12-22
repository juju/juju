// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"
)

type description struct {
	// Version conceptually encapsulates an understanding of which fields
	// exist and how they are populated. As extra fields and entities are
	// added, the version should be incremented and tests written to ensure
	// that newer versions of the code are still able to create Model
	// representations from versions.
	//
	// The version is all about the serialization of the structures from
	// the migration package. Each type will likely have a version.
	version int
	model   *model
	// TODO: extra binaries...
	// Tools
	// Charms
}

func (d *description) Model() Model {
	return d.model
}

type model struct {
	Version int `yaml:"version"`

	UUID   string `yaml:"model-uuid"`
	Name_  string `yaml:"name"`
	Owner_ string `yaml:"owner"`

	Machines_ machines `yaml:"machines"`

	// TODO: add extra entities, but initially focus on Machines.
	// Services, and through them, Units
	// Relations
	// Spaces
	// Storage

}

type machines struct {
	Version   int        `yaml:"version"`
	Machines_ []*machine `yaml:"machines"`
}

func (m *model) Id() names.EnvironTag {
	return names.NewEnvironTag(m.UUID)
}

func (m *model) Name() string {
	return m.Name_
}

func (m *model) Owner() names.UserTag {
	return names.NewUserTag(m.Owner_)
}

func (m *model) Machines() []Machine {
	var result []Machine
	for _, machine := range m.Machines_.Machines_ {
		result = append(result, machine)
	}
	return result
}

func (m *model) setMachines(machineList []*machine) {
	m.Machines_ = machines{
		Version:   1,
		Machines_: machineList,
	}
}

type machine struct {
	Id_         string     `yaml:"id"`
	Containers_ []*machine `yaml:"containers"`
}

func (m *machine) Id() names.MachineTag {
	return names.NewMachineTag(m.Id_)
}

func (m *machine) Containers() []Machine {
	var result []Machine
	for _, container := range m.Containers_ {
		result = append(result, container)
	}
	return result
}
