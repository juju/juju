// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/names/v6"
	"github.com/juju/schema"

	"github.com/juju/juju/apiserver/common/storagecommon"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/state"
)

// Backend defines the state functionality required by the application
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	Application(string) (Application, error)
	ApplyOperation(state.ModelOperation) error
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)
	Machine(string) (Machine, error)
	Unit(string) (Unit, error)

	// ReadSequence is a stop gap to allow the next unit number to be read from mongo
	// so that correctly matching units can be written to dqlite.
	ReadSequence(name string) (int, error)
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	Name() string
	AddUnit(state.AddUnitParams) (Unit, error)
	AllUnits() ([]Unit, error)
	ApplicationTag() names.ApplicationTag
	CharmURL() (*string, bool)
	CharmOrigin() *state.CharmOrigin
	DestroyOperation(objectstore.ObjectStore) *state.DestroyApplicationOperation
	EndpointBindings() (Bindings, error)
	Endpoints() ([]relation.Endpoint, error)
	IsPrincipal() bool
	IsRemote() bool
	SetCharm(state.SetCharmConfig, objectstore.ObjectStore) error
	SetConstraints(constraints.Value) error
	UpdateCharmConfig(charm.Settings) error
	UpdateApplicationConfig(coreconfig.ConfigAttributes, []string, configschema.Fields, schema.Defaults) error
	MergeBindings(*state.Bindings, bool) error
}

// Bindings defines a subset of the functionality provided by the
// state.Bindings type, as required by the application facade. For
// details on the methods, see the methods on state.Bindings with
// the same names.
type Bindings interface {
	Map() map[string]string
	MapWithSpaceNames(network.SpaceInfos) (map[string]string, error)
}

// Charm defines a subset of the functionality provided by the
// state.Charm type, as required by the application facade. For
// details on the methods, see the methods on state.Charm with
// the same names.
type Charm interface {
	CharmMeta
	Config() *charm.Config
	Actions() *charm.Actions
	Revision() int
	URL() string
	Version() string
}

// CharmMeta describes methods that inform charm operation.
type CharmMeta interface {
	Manifest() *charm.Manifest
	Meta() *charm.Meta
}

// Machine defines a subset of the functionality provided by the
// state.Machine type, as required by the application facade. For
// details on the methods, see the methods on state.Machine with
// the same names.
type Machine interface {
	Base() state.Base
	Id() string
	PublicAddress() (network.SpaceAddress, error)
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	Name() string
	Tag() names.Tag
	UnitTag() names.UnitTag
	ApplicationName() string
	DestroyOperation(objectstore.ObjectStore) *state.DestroyUnitOperation
	IsPrincipal() bool

	AssignedMachineId() (string, error)
	WorkloadVersion() (string, error)
	AssignUnit() error
	AssignWithPlacement(*instance.Placement, network.SpaceInfos) error
	ContainerInfo() (state.CloudContainer, error)
}

type stateShim struct {
	*state.State
}

type modelShim struct {
	*state.Model
}

type StorageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State) (StorageInterface, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	storageAccess := &storageShim{
		StorageAccess: sb,
		va:            sb,
		fa:            sb,
	}
	// CAAS models don't support volume storage yet.
	if m.Type() == state.ModelTypeCAAS {
		storageAccess.va = nil
	}
	return storageAccess, nil
}

type storageShim struct {
	storagecommon.StorageAccess
	fa storagecommon.FilesystemAccess
	va storagecommon.VolumeAccess
}

func (s *storageShim) VolumeAccess() storagecommon.VolumeAccess {
	return s.va
}

func (s *storageShim) FilesystemAccess() storagecommon.FilesystemAccess {
	return s.fa
}

// NewStateApplication converts a state.Application into an Application.
func NewStateApplication(
	st *state.State,
	app *state.Application,
) Application {
	return stateApplicationShim{
		Application: app,
		st:          st,
	}
}

func (s stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{
		Application: a,
		st:          s.State,
	}, nil
}

func (s stateShim) ReadSequence(name string) (int, error) {
	return state.ReadSequence(s.State, name)
}

func (s stateShim) AddApplication(args state.AddApplicationArgs, store objectstore.ObjectStore) (Application, error) {
	a, err := s.State.AddApplication(args, store)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{
		Application: a,
		st:          s.State,
	}, nil
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return stateMachineShim{Machine: m}, nil
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return stateUnitShim{
		Unit: u,
		st:   s.State,
	}, nil
}

type stateApplicationShim struct {
	*state.Application
	st *state.State
}

func (a stateApplicationShim) AddUnit(args state.AddUnitParams) (Unit, error) {
	u, err := a.Application.AddUnit(args)
	if err != nil {
		return nil, err
	}
	return stateUnitShim{
		Unit: u,
		st:   a.st,
	}, nil
}

func (a stateApplicationShim) AllUnits() ([]Unit, error) {
	units, err := a.Application.AllUnits()
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(units))
	for i, u := range units {
		out[i] = stateUnitShim{
			Unit: u,
			st:   a.st,
		}
	}
	return out, nil
}

func (a stateApplicationShim) EndpointBindings() (Bindings, error) {
	return a.Application.EndpointBindings()
}

func (a stateApplicationShim) SetCharm(
	config state.SetCharmConfig,
	objStore objectstore.ObjectStore,
) error {
	return a.Application.SetCharm(config, objStore)
}

type stateMachineShim struct {
	*state.Machine
}

type stateUnitShim struct {
	*state.Unit
	st *state.State
}

func (u stateUnitShim) AssignUnit() error {
	return u.st.AssignUnit(u.Unit)
}

func (u stateUnitShim) AssignWithPlacement(placement *instance.Placement, allSpaces network.SpaceInfos) error {
	return u.st.AssignUnitWithPlacement(u.Unit, placement, allSpaces)
}
