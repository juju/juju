// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// Unit represents a state.Unit.
type Unit interface {
	status.StatusHistoryGetter
	Life() state.Life
	Destroy() (err error)
	IsPrincipal() bool
	PublicAddress() (network.Address, error)
	PrivateAddress() (network.Address, error)
	Resolve(retryHooks bool) error
	AgentHistory() status.StatusHistoryGetter
}

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	FindEntity(names.Tag) (state.Entity, error)
	Unit(string) (Unit, error)
	Application(string) (*state.Application, error)
	Machine(string) (*state.Machine, error)
	AllMachines() ([]*state.Machine, error)
	AllApplications() ([]*state.Application, error)
	AllRelations() ([]*state.Relation, error)
	AddOneMachine(state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideMachine(state.MachineTemplate, string, instance.ContainerType) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	ModelConstraints() (constraints.Value, error)
	ModelConfig() (*config.Config, error)
	ModelConfigValues() (config.ConfigValues, error)
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
	AddModelUser(string, state.UserAccessSpec) (permission.UserAccess, error)
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	Watch() *state.Multiwatcher
	AbortCurrentUpgrade() error
	APIHostPorts() ([][]network.HostPort, error)
	LatestMigration() (state.ModelMigration, error)
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return u, nil
}
