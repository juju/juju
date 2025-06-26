// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/schema"

	"github.com/juju/juju/apiserver/common/storagecommon"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
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
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	AddUnit(state.AddUnitParams) (Unit, error)
	DestroyOperation(objectstore.ObjectStore) *state.DestroyApplicationOperation
	SetCharm(state.SetCharmConfig, objectstore.ObjectStore) error
	SetConstraints(constraints.Value) error
	UpdateCharmConfig(charm.Settings) error
	UpdateApplicationConfig(coreconfig.ConfigAttributes, []string, configschema.Fields, schema.Defaults) error
}

// Bindings defines a subset of the functionality provided by the
// state.Bindings type, as required by the application facade. For
// details on the methods, see the methods on state.Bindings with
// the same names.
type Bindings interface {
	Map() map[string]network.SpaceUUID
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
	DestroyOperation(objectstore.ObjectStore) *state.DestroyUnitOperation

	AssignUnit() error
	AssignWithPlacement(*instance.Placement, network.SpaceInfos) error
}

type stateShim struct {
	*state.State
}
type StorageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State, modelType coremodel.ModelType) (StorageInterface, error) {
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
	if modelType == coremodel.CAAS {
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
	return m, nil
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
