// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// Backend defines the state functionality required by the application
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	AllModels() ([]Model, error)
	Application(string) (Application, error)
	AddApplication(state.AddApplicationArgs) (*state.Application, error)
	RemoteApplication(name string) (*state.RemoteApplication, error)
	AddRemoteApplication(args state.AddRemoteApplicationParams) (*state.RemoteApplication, error)
	AddRelation(...state.Endpoint) (Relation, error)
	AssignUnit(*state.Unit, state.AssignmentPolicy) error
	AssignUnitWithPlacement(*state.Unit, *instance.Placement) error
	Charm(*charm.URL) (Charm, error)
	EndpointsRelation(...state.Endpoint) (Relation, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	Machine(string) (Machine, error)
	ModelTag() names.ModelTag
	Unit(string) (Unit, error)
	NewStorage() storage.Storage
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
	GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error)
}

// BlockChecker defines the block-checking functionality required by
// the application facade. This is implemented by
// apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	AddUnit() (*state.Unit, error)
	AllUnits() ([]Unit, error)
	Charm() (Charm, bool, error)
	CharmURL() (*charm.URL, bool)
	Channel() csparams.Channel
	ClearExposed() error
	ConfigSettings() (charm.Settings, error)
	Constraints() (constraints.Value, error)
	Destroy() error
	Endpoints() ([]state.Endpoint, error)
	IsPrincipal() bool
	Series() string
	SetCharm(state.SetCharmConfig) error
	SetConstraints(constraints.Value) error
	SetExposed() error
	SetMetricCredentials([]byte) error
	SetMinUnits(int) error
	UpdateConfigSettings(charm.Settings) error
}

// Charm defines a subset of the functionality provided by the
// state.Charm type, as required by the application facade. For
// details on the methods, see the methods on state.Charm with
// the same names.
type Charm interface {
	charm.Charm
	StoragePath() string
}

// Machine defines a subset of the functionality provided by the
// state.Machine type, as required by the application facade. For
// details on the methods, see the methods on state.Machine with
// the same names.
type Machine interface {
}

// Relation defines a subset of the functionality provided by the
// state.Relation type, as required by the application facade. For
// details on the methods, see the methods on state.Relation with
// the same names.
type Relation interface {
	Destroy() error
	Endpoint(string) (state.Endpoint, error)
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	UnitTag() names.UnitTag
	Destroy() error
	IsPrincipal() bool
	Life() state.Life
}

// Model defines a subset of the functionality provided by the
// state.Model type, as required by the application facade. For
// details on the methods, see the methods on state.Model with
// the same names.
type Model interface {
	Tag() names.Tag
	Name() string
	Owner() names.UserTag
}

type stateShim struct {
	*state.State
}

// NewStateBackend converts a state.State into a Backend.
func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

// CharmToStateCharm converts a Charm into a state.Charm. This is
// a hack that is required until the State interface methods we
// deal with stop accepting state.Charms, and start accepting
// charm.Charm and charm.URL.
func CharmToStateCharm(ch Charm) *state.Charm {
	return ch.(stateCharmShim).Charm
}

func (s stateShim) NewStorage() storage.Storage {
	return storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
}

func (s stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{a}, nil
}

func (s stateShim) AddRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := s.State.AddRelation(eps...)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r}, nil
}

func (s stateShim) Charm(curl *charm.URL) (Charm, error) {
	ch, err := s.State.Charm(curl)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{ch}, nil
}

func (s stateShim) EndpointsRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := s.State.EndpointsRelation(eps...)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r}, nil
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return stateMachineShim{m}, nil
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return stateUnitShim{u}, nil
}

func (s stateShim) AllModels() ([]Model, error) {
	models, err := s.State.AllModels()
	if err != nil {
		return nil, err
	}
	result := make([]Model, len(models))
	for i, m := range models {
		result[i] = stateModelShim{m}
	}
	return result, nil
}

type stateApplicationShim struct {
	*state.Application
}

func (a stateApplicationShim) Charm() (Charm, bool, error) {
	ch, force, err := a.Application.Charm()
	if err != nil {
		return nil, false, err
	}
	return ch, force, nil
}

func (a stateApplicationShim) AllUnits() ([]Unit, error) {
	units, err := a.Application.AllUnits()
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(units))
	for i, u := range units {
		out[i] = stateUnitShim{u}
	}
	return out, nil
}

type stateCharmShim struct {
	*state.Charm
}

type stateMachineShim struct {
	*state.Machine
}

type stateRelationShim struct {
	*state.Relation
}

type stateUnitShim struct {
	*state.Unit
}

type stateModelShim struct {
	*state.Model
}
