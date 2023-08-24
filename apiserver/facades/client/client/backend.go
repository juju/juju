// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	network.SpaceLookup

	AbortCurrentUpgrade() error
	AddControllerUser(state.UserAccessSpec) (permission.UserAccess, error)
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
	AllSubnets() ([]*state.Subnet, error)
	Annotations(state.GlobalEntity) (map[string]string, error)
	APIHostPortsForClients(controller.Config) ([]network.SpaceHostPorts, error)
	Application(string) (Application, error)
	Charm(*charm.URL) (*state.Charm, error)
	LegacyControllerConfig() (controller.Config, error)
	ControllerNodes() ([]state.ControllerNode, error)
	ControllerTag() names.ControllerTag
	ControllerTimestamp() (*time.Time, error)
	EndpointsRelation(...state.Endpoint) (*state.Relation, error)
	FindEntity(names.Tag) (state.Entity, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	IsController() bool
	HAPrimaryMachine() (names.MachineTag, error)
	LatestMigration() (state.ModelMigration, error)
	LatestPlaceholderCharm(*charm.URL) (*state.Charm, error)
	Machine(string) (*state.Machine, error)
	Model() (Model, error)
	ModelConfig() (*config.Config, error)
	ModelConstraints() (constraints.Value, error)
	ModelTag() names.ModelTag
	ModelUUID() string
	MongoSession() MongoSession
	RemoteApplication(string) (*state.RemoteApplication, error)
	RemoteConnectionStatus(string) (*state.RemoteConnectionStatus, error)
	RemoveUserAccess(names.UserTag, names.Tag) error
	SetAnnotations(state.GlobalEntity, map[string]string) error
	SetModelAgentVersion(version.Number, *string, bool) error
	SetModelConstraints(constraints.Value) error
	Unit(string) (Unit, error)
	UpdateModelConfig(map[string]interface{}, []string, ...state.ValidateConfigFunc) error
}

// MongoSession provides a way to get the status for the mongo replicaset.
type MongoSession interface {
	CurrentStatus() (*replicaset.Status, error)
}

// Model contains the state.Model methods used in this package.
type Model interface {
	Name() string
	Type() state.ModelType
	UUID() string
	Life() state.Life
	CloudName() string
	CloudRegion() string
	CloudCredentialTag() (names.CloudCredentialTag, bool)
	Config() (*config.Config, error)
	Owner() names.UserTag
	AddUser(state.UserAccessSpec) (permission.UserAccess, error)
	Users() ([]permission.UserAccess, error)
	StatusHistory(status.StatusHistoryFilter) ([]status.StatusInfo, error)
	SLAOwner() string
	SLALevel() string
	LatestToolsVersion() version.Number
	MeterStatus() state.MeterStatus
	Status() (status.StatusInfo, error)
}

// Pool contains the StatePool functionality used in this package.
type Pool interface {
	GetModel(string) (*state.Model, func(), error)
	SystemState() (*state.State, error)
}

// Application represents a state.Application.
type Application interface {
	StatusHistory(status.StatusHistoryFilter) ([]status.StatusInfo, error)
}

// Unit represents a state.Unit.
type Unit interface {
	status.StatusHistoryGetter
	Life() state.Life
	Destroy() (err error)
	IsPrincipal() bool
	PublicAddress() (network.SpaceAddress, error)
	PrivateAddress() (network.SpaceAddress, error)
	Resolve(retryHooks bool) error
	AgentHistory() status.StatusHistoryGetter
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	model   *state.Model
	session MongoSession
}

func (s stateShim) UpdateModelConfig(u map[string]interface{}, r []string, a ...state.ValidateConfigFunc) error {
	return s.model.UpdateModelConfig(u, r, a...)
}

func (s *stateShim) Annotations(entity state.GlobalEntity) (map[string]string, error) {
	return s.model.Annotations(entity)
}

func (s *stateShim) SetAnnotations(entity state.GlobalEntity, ann map[string]string) error {
	return s.model.SetAnnotations(entity, ann)
}

func (s *stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *stateShim) AllApplicationOffers() ([]*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.State)
	return offers.AllApplicationOffers()
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) SystemState() (*state.State, error) {
	return p.pool.SystemState()
}

func (p *poolShim) GetModel(uuid string) (*state.Model, func(), error) {
	model, ph, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { ph.Release() }, nil
}

func (s stateShim) ModelConfig() (*config.Config, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := model.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cfg, nil
}

func (s stateShim) ModelTag() names.ModelTag {
	return names.NewModelTag(s.State.ModelUUID())
}

func (s stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &modelShim{m}, nil
}

func (s stateShim) ControllerNodes() ([]state.ControllerNode, error) {
	nodes, err := s.State.ControllerNodes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]state.ControllerNode, len(nodes))
	for i, n := range nodes {
		result[i] = n
	}
	return result, nil
}

func (s stateShim) MongoSession() MongoSession {
	if s.session != nil {
		return s.session
	}
	return MongoSessionShim{s.State.MongoSession()}
}

type modelShim struct {
	*state.Model
}

// MongoSessionShim wraps a *mgo.Session to conform to the
// MongoSession interface.
type MongoSessionShim struct {
	*mgo.Session
}

// CurrentStatus returns the current status of the replicaset.
func (s MongoSessionShim) CurrentStatus() (*replicaset.Status, error) {
	return replicaset.CurrentStatus(s.Session)
}
