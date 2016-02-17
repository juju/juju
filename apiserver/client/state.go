// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

// Unit represents a state.Unit.
type Unit interface {
	state.StatusHistoryGetter
	Life() state.Life
	Destroy() (err error)
	IsPrincipal() bool
	PublicAddress() (network.Address, error)
	PrivateAddress() (network.Address, error)
	Resolve(retryHooks bool) error
	AgentHistory() state.StatusHistoryGetter
}

// stateInterface contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type stateInterface interface {
	FindEntity(names.Tag) (state.Entity, error)
	Unit(string) (Unit, error)
	Service(string) (*state.Service, error)
	Machine(string) (*state.Machine, error)
	AllMachines() ([]*state.Machine, error)
	AllServices() ([]*state.Service, error)
	AllRelations() ([]*state.Relation, error)
	AllNetworks() ([]*state.Network, error)
	AddOneMachine(state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideMachine(state.MachineTemplate, string, instance.ContainerType) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	ModelConstraints() (constraints.Value, error)
	ModelConfig() (*config.Config, error)
	UpdateModelConfig(map[string]interface{}, []string, state.ValidateConfigFunc) error
	SetModelConstraints(constraints.Value) error
	ModelUUID() string
	ModelTag() names.ModelTag
	Model() (*state.Model, error)
	ForModel(tag names.ModelTag) (*state.State, error)
	SetModelAgentVersion(version.Number) error
	SetAnnotations(state.GlobalEntity, map[string]string) error
	Annotations(state.GlobalEntity) (map[string]string, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	EndpointsRelation(...state.Endpoint) (*state.Relation, error)
	Charm(*charm.URL) (*state.Charm, error)
	LatestPlaceholderCharm(*charm.URL) (*state.Charm, error)
	AddRelation(...state.Endpoint) (*state.Relation, error)
	AddModelUser(state.ModelUserSpec) (*state.ModelUser, error)
	RemoveModelUser(names.UserTag) error
	Watch() *state.Multiwatcher
	AbortCurrentUpgrade() error
	APIHostPorts() ([][]network.HostPort, error)
}

type stateShim struct {
	*state.State
}

func (s *stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return u, nil
}
