// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

// HistoricalUnit represents a state.Unit instance that
// can fetch its status history.
type HistoricalUnit interface {
	state.StatusHistoryGetter
	AgentHistoryGetter() state.StatusHistoryGetter
}

// stateInterface contains the state.State methods used in this package,
// allowing stubs to ve created for testing.
type stateInterface interface {
	FindEntity(names.Tag) (state.Entity, error)
	Unit(string) (*state.Unit, error)
	HistoricalUnit(string) (HistoricalUnit, error)
	Service(string) (*state.Service, error)
	Machine(string) (*state.Machine, error)
	AllMachines() ([]*state.Machine, error)
	AllServices() ([]*state.Service, error)
	AllRelations() ([]*state.Relation, error)
	AllNetworks() ([]*state.Network, error)
	AddOneMachine(state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideMachine(state.MachineTemplate, string, instance.ContainerType) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	EnvironConstraints() (constraints.Value, error)
	EnvironConfig() (*config.Config, error)
	UpdateEnvironConfig(map[string]interface{}, []string, state.ValidateConfigFunc) error
	SetEnvironConstraints(constraints.Value) error
	EnvironUUID() string
	EnvironTag() names.EnvironTag
	Environment() (*state.Environment, error)
	SetEnvironAgentVersion(version.Number) error
	SetAnnotations(state.GlobalEntity, map[string]string) error
	Annotations(state.GlobalEntity) (map[string]string, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	EndpointsRelation(...state.Endpoint) (*state.Relation, error)
	Charm(*charm.URL) (*state.Charm, error)
	LatestPlaceholderCharm(*charm.URL) (*state.Charm, error)
	AddRelation(...state.Endpoint) (*state.Relation, error)
	AddEnvironmentUser(user, createdBy names.UserTag, displayName string) (*state.EnvironmentUser, error)
	RemoveEnvironmentUser(names.UserTag) error
	Watch() *state.Multiwatcher
	AbortCurrentUpgrade() error
	APIHostPorts() ([][]network.HostPort, error)
}

type stateShim struct {
	*state.State
}

func (s *stateShim) HistoricalUnit(name string) (HistoricalUnit, error) {
	u, err := s.Unit(name)
	if err != nil {
		return nil, err
	}
	return &historicalUnit{u}, nil
}

type historicalUnit struct {
	*state.Unit
}

func (h *historicalUnit) AgentHistoryGetter() state.StatusHistoryGetter {
	return h.Agent()
}
