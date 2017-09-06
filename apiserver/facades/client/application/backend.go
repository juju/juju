// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// Backend defines the state functionality required by the application
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	storagecommon.StorageInterface

	AllModelUUIDs() ([]string, error)
	Application(string) (Application, error)
	AddApplication(state.AddApplicationArgs) (Application, error)
	RemoteApplication(string) (RemoteApplication, error)
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)
	AddRelation(...state.Endpoint) (Relation, error)
	Charm(*charm.URL) (Charm, error)
	EndpointsRelation(...state.Endpoint) (Relation, error)
	Relation(int) (Relation, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	Machine(string) (Machine, error)
	ModelTag() names.ModelTag
	Unit(string) (Unit, error)
	SaveController(info crossmodel.ControllerInfo, modelUUID string) (ExternalController, error)
	ControllerTag() names.ControllerTag
	Resources() (Resources, error)
	OfferConnectionForRelation(string) (OfferConnection, error)
	SaveEgressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error)
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
	AddUnit(state.AddUnitParams) (Unit, error)
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
	UpdateApplicationSeries(string, bool) error
	UpdateConfigSettings(charm.Settings) error
}

// Charm defines a subset of the functionality provided by the
// state.Charm type, as required by the application facade. For
// details on the methods, see the methods on state.Charm with
// the same names.
type Charm interface {
	charm.Charm
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
	status.StatusSetter
	Tag() names.Tag
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

	AssignWithPolicy(state.AssignmentPolicy) error
	AssignWithPlacement(*instance.Placement) error
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

// Resources defines a subset of the functionality provided by the
// state.Resources type, as required by the application facade. See
// the state.Resources type for details on the methods.
type Resources interface {
	RemovePendingAppResources(string, map[string]string) error
}

type stateShim struct {
	*state.State
	*state.IAASModel
}

type ExternalController state.ExternalController

func (s stateShim) SaveController(controllerInfo crossmodel.ControllerInfo, modelUUID string) (ExternalController, error) {
	api := state.NewExternalControllers(s.State)
	return api.Save(controllerInfo, modelUUID)
}

// NewStateBackend converts a state.State into a Backend.
func NewStateBackend(st *state.State) (Backend, error) {
	im, err := st.IAASModel()
	if err != nil {
		return nil, err
	}
	return &stateShim{
		State:     st,
		IAASModel: im,
	}, nil
}

// NewStateApplication converts a state.Application into an Application.
func NewStateApplication(st *state.State, app *state.Application) Application {
	return stateApplicationShim{app, st}
}

// CharmToStateCharm converts a Charm into a state.Charm. This is
// a hack that is required until the State interface methods we
// deal with stop accepting state.Charms, and start accepting
// charm.Charm and charm.URL.
func CharmToStateCharm(ch Charm) *state.Charm {
	return ch.(stateCharmShim).Charm
}

func (s stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{a, s.State}, nil
}

func (s stateShim) AddApplication(args state.AddApplicationArgs) (Application, error) {
	a, err := s.State.AddApplication(args)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{a, s.State}, nil
}

type remoteApplicationShim struct {
	*state.RemoteApplication
}

type RemoteApplication interface {
	Name() string
	SourceModel() names.ModelTag
	Endpoints() ([]state.Endpoint, error)
	AddEndpoints(eps []charm.Relation) error
	Bindings() map[string]string
	Spaces() []state.RemoteSpace
	Destroy() error
}

func (s stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	app, err := s.State.RemoteApplication(name)
	return &remoteApplicationShim{app}, err
}

func (s stateShim) AddRemoteApplication(args state.AddRemoteApplicationParams) (RemoteApplication, error) {
	app, err := s.State.AddRemoteApplication(args)
	return &remoteApplicationShim{app}, err
}

func (s stateShim) AddRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := s.State.AddRelation(eps...)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r}, nil
}

func (s stateShim) SaveEgressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error) {
	api := state.NewRelationEgressNetworks(s.State)
	return api.Save(relationKey, false, cidrs)
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

func (s stateShim) Relation(id int) (Relation, error) {
	r, err := s.State.Relation(id)
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
	return stateUnitShim{u, s.State}, nil
}

func (s stateShim) Resources() (Resources, error) {
	return s.State.Resources()
}

type OfferConnection interface{}

func (s stateShim) OfferConnectionForRelation(key string) (OfferConnection, error) {
	return s.State.OfferConnectionForRelation(key)
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
	return stateUnitShim{u, a.st}, nil
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
		out[i] = stateUnitShim{u, a.st}
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
	st *state.State
}

func (u stateUnitShim) AssignWithPolicy(policy state.AssignmentPolicy) error {
	return u.st.AssignUnit(u.Unit, policy)
}

func (u stateUnitShim) AssignWithPlacement(placement *instance.Placement) error {
	return u.st.AssignUnitWithPlacement(u.Unit, placement)
}

type Subnet interface {
	CIDR() string
	VLANTag() int
	ProviderId() network.Id
	ProviderNetworkId() network.Id
}

type subnetShim struct {
	*state.Subnet
}

type Space interface {
	Name() string
	ProviderId() network.Id
}

type spaceShim struct {
	*state.Space
}
