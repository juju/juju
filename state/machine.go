// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/tools"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
}

// machineDoc represents the internal state of a machine in MongoDB.
// Note the correspondence with MachineInfo in apiserver/juju.
type machineDoc struct {
	DocID         string `bson:"_id"`
	Id            string `bson:"machineid"`
	Base          Base   `bson:"base"`
	ContainerType string
	Principals    []string
	Life          Life
	Tools         *tools.Tools `bson:",omitempty"`

	// Volumes contains the names of volumes attached to the machine.
	Volumes []string `bson:"volumes,omitempty"`
	// Filesystems contains the names of filesystems attached to the machine.
	Filesystems []string `bson:"filesystems,omitempty"`

	// PreferredPublicAddress is the preferred address to be used for
	// the machine when a public address is requested.
	PreferredPublicAddress address `bson:",omitempty"`
}

func newMachine(st *State, doc *machineDoc) *Machine {
	machine := &Machine{
		st:  st,
		doc: *doc,
	}
	return machine
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// Principals returns the principals for the machine.
func (m *Machine) Principals() []string {
	return m.doc.Principals
}

// Base returns the os base running on the machine.
func (m *Machine) Base() Base {
	return m.doc.Base
}

// ContainerType returns the type of container hosting this machine.
func (m *Machine) ContainerType() instance.ContainerType {
	return instance.ContainerType(m.doc.ContainerType)
}

// Tag returns a tag identifying the machine. The String method provides a
// string representation that is safe to use as a file name. The returned name
// will be different from other Tag values returned by any other entities
// from the same state.
func (m *Machine) Tag() names.Tag {
	return names.NewMachineTag(m.Id())
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// AgentTools returns the tools that the agent is currently running.
// It returns an error that satisfies errors.IsNotFound if the tools
// have not yet been set.
func (m *Machine) AgentTools() (*tools.Tools, error) {
	if m.doc.Tools == nil {
		return nil, errors.NotFoundf("agent binaries for machine %v", m)
	}
	tools := *m.doc.Tools
	return &tools, nil
}

// PublicAddress returns a public address for the machine. If no address is
// available it returns an error that satisfies network.IsNoAddressError().
func (m *Machine) PublicAddress() (network.SpaceAddress, error) {
	publicAddress := m.doc.PreferredPublicAddress.networkAddress()
	var err error
	if publicAddress.Value == "" {
		err = network.NoAddressError("public")
	}
	return publicAddress, err
}
