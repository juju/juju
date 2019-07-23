// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/schema"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// Backend defines the state functionality required by the application
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	AllModelUUIDs() ([]string, error)
	Application(string) (Application, error)
	ApplyOperation(state.ModelOperation) error
	AddApplication(state.AddApplicationArgs) (Application, error)
	RemoteApplication(string) (RemoteApplication, error)
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)
	AddRelation(...state.Endpoint) (Relation, error)
	Charm(*charm.URL) (Charm, error)
	EndpointsRelation(...state.Endpoint) (Relation, error)
	Relation(int) (Relation, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	Machine(string) (Machine, error)
	Unit(string) (Unit, error)
	UnitsInError() ([]Unit, error)
	SaveController(info crossmodel.ControllerInfo, modelUUID string) (ExternalController, error)
	ControllerTag() names.ControllerTag
	Resources() (Resources, error)
	OfferConnectionForRelation(string) (OfferConnection, error)
	SaveEgressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error)
	Branch(string) (Generation, error)
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
	ApplicationConfig() (application.ConfigAttributes, error)
	Charm() (Charm, bool, error)
	CharmURL() (*charm.URL, bool)
	Channel() csparams.Channel
	ClearExposed() error
	CharmConfig(string) (charm.Settings, error)
	Constraints() (constraints.Value, error)
	Destroy() error
	DestroyOperation() *state.DestroyApplicationOperation
	EndpointBindings() (map[string]string, error)
	Endpoints() ([]state.Endpoint, error)
	IsExposed() bool
	IsPrincipal() bool
	IsRemote() bool
	Series() string
	SetCharm(state.SetCharmConfig) error
	SetConstraints(constraints.Value) error
	SetExposed() error
	SetMetricCredentials([]byte) error
	SetMinUnits(int) error
	UpdateApplicationSeries(string, bool) error
	UpdateCharmConfig(string, charm.Settings) error
	UpdateApplicationConfig(application.ConfigAttributes, []string, environschema.Fields, schema.Defaults) error
	SetScale(int, int64, bool) error
	ChangeScale(int) (int, error)
	AgentTools() (*tools.Tools, error)
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
	IsLockedForSeriesUpgrade() (bool, error)
	IsParentLockedForSeriesUpgrade() (bool, error)
}

// Relation defines a subset of the functionality provided by the
// state.Relation type, as required by the application facade. For
// details on the methods, see the methods on state.Relation with
// the same names.
type Relation interface {
	status.StatusSetter
	Tag() names.Tag
	Destroy() error
	DestroyWithForce(bool, time.Duration) ([]error, error)
	Endpoint(string) (state.Endpoint, error)
	SetSuspended(bool, string) error
	Suspended() bool
	SuspendedReason() string
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	Name() string
	Tag() names.Tag
	UnitTag() names.UnitTag
	Destroy() error
	DestroyOperation() *state.DestroyUnitOperation
	IsPrincipal() bool
	Life() state.Life
	Resolve(retryHooks bool) error
	AgentTools() (*tools.Tools, error)

	AssignedMachineId() (string, error)
	AssignWithPolicy(state.AssignmentPolicy) error
	AssignWithPlacement(*instance.Placement) error
}

// Model defines a subset of the functionality provided by the
// state.Model type, as required by the application facade. For
// details on the methods, see the methods on state.Model with
// the same names.
type Model interface {
	ModelTag() names.ModelTag
	Name() string
	Owner() names.UserTag
	Tag() names.Tag
	Type() state.ModelType
	ModelConfig() (*config.Config, error)
	AgentVersion() (version.Number, error)
}

// Resources defines a subset of the functionality provided by the
// state.Resources type, as required by the application facade. See
// the state.Resources type for details on the methods.
type Resources interface {
	RemovePendingAppResources(string, map[string]string) error
}

type Generation interface {
	AssignApplication(string) error
}

type stateShim struct {
	*state.State
}

type ExternalController state.ExternalController

func (s stateShim) SaveController(controllerInfo crossmodel.ControllerInfo, modelUUID string) (ExternalController, error) {
	api := state.NewExternalControllers(s.State)
	return api.Save(controllerInfo, modelUUID)
}

type storageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State) (storageInterface, error) {
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
	DestroyOperation(force bool) *state.DestroyRemoteApplicationOperation
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

func (s stateShim) UnitsInError() ([]Unit, error) {
	units, err := s.State.UnitsInError()
	if err != nil {
		return nil, err
	}
	result := make([]Unit, len(units))
	for i, u := range units {
		result[i] = stateUnitShim{u, s.State}
	}
	return result, nil
}

func (s stateShim) Resources() (Resources, error) {
	return s.State.Resources()
}

type OfferConnection interface{}

func (s stateShim) OfferConnectionForRelation(key string) (OfferConnection, error) {
	return s.State.OfferConnectionForRelation(key)
}

func (s stateShim) Branch(name string) (Generation, error) {
	gen, err := s.State.Branch(name)
	if err != nil {
		return nil, err
	}
	return Generation(gen), nil
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

type Space interface {
	Name() string
	ProviderId() network.Id
}
