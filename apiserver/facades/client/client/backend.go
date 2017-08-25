// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	AbortCurrentUpgrade() error
	AddMachineInsideMachine(state.MachineTemplate, string, instance.ContainerType) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddOneMachine(state.MachineTemplate) (*state.Machine, error)
	AddRelation(...state.Endpoint) (*state.Relation, error)
	AllApplications() ([]*state.Application, error)
	AllApplicationOffers() ([]*crossmodel.ApplicationOffer, error)
	AllRemoteApplications() ([]*state.RemoteApplication, error)
	AllMachines() ([]*state.Machine, error)
	AllModelUUIDs() ([]string, error)
	AllIPAddresses() ([]*state.Address, error)
	AllLinkLayerDevices() ([]*state.LinkLayerDevice, error)
	AllRelations() ([]*state.Relation, error)
	Annotations(state.GlobalEntity) (map[string]string, error)
	APIHostPorts() ([][]network.HostPort, error)
	Application(string) (*state.Application, error)
	ApplicationLeaders() (map[string]string, error)
	Charm(*charm.URL) (*state.Charm, error)
	ControllerTag() names.ControllerTag
	EndpointsRelation(...state.Endpoint) (*state.Relation, error)
	FindEntity(names.Tag) (state.Entity, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	IsController() bool
	LatestMigration() (state.ModelMigration, error)
	LatestPlaceholderCharm(*charm.URL) (*state.Charm, error)
	Machine(string) (*state.Machine, error)
	Model() (*state.Model, error)
	ModelConfig() (*config.Config, error)
	ModelConfigValues() (config.ConfigValues, error)
	ModelConstraints() (constraints.Value, error)
	ModelTag() names.ModelTag
	ModelUUID() string
	RemoteApplication(string) (*state.RemoteApplication, error)
	RemoteConnectionStatus(string) (*state.RemoteConnectionStatus, error)
	SetAnnotations(state.GlobalEntity, map[string]string) error
	SetModelAgentVersion(version.Number) error
	SetModelConstraints(constraints.Value) error
	Subnet(string) (*state.Subnet, error)
	Unit(string) (Unit, error)
	UpdateModelConfig(map[string]interface{}, []string, ...state.ValidateConfigFunc) error
	Watch(params state.WatchParams) *state.Multiwatcher
}

// Model contains the state.Model methods used in this package.
type Model interface {
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
	AddModelUser(string, state.UserAccessSpec) (permission.UserAccess, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
}

// Pool contains the StatePool functionality used in this package.
type Pool interface {
	GetModel(string) (*state.Model, func(), error)
}

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

func (s *stateShim) Watch(params state.WatchParams) *state.Multiwatcher {
	return s.State.Watch(params)
}

func (s *stateShim) AllApplicationOffers() ([]*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.State)
	return offers.AllApplicationOffers()
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) GetModel(uuid string) (*state.Model, func(), error) {
	model, release, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { release() }, nil
}
