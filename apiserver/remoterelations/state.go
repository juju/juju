// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// StatePool provides the subset of a state pool required by the
// remote relations facade.
type StatePool interface {
	// Get returns a State for a given model from the pool.
	Get(modelUUID string) (RemoteRelationsState, func(), error)
}

// RemoteRelationState provides the subset of global state required by the
// remote relations facade.
type RemoteRelationsState interface {
	// ModelUUID returns the model UUID for the model
	// controlled by this state instance.
	ModelUUID() string

	// AllSubnets returns all known subnets in the model.
	AllSubnets() (subnets []Subnet, err error)

	// KeyRelation returns the existing relation with the given key (which can
	// be derived unambiguously from the relation's endpoints).
	KeyRelation(string) (Relation, error)

	// AddRelation adds a relation between the specified endpoints and returns the relation info.
	AddRelation(...state.Endpoint) (Relation, error)

	// EndpointsRelation returns the existing relation with the given endpoints.
	EndpointsRelation(...state.Endpoint) (Relation, error)

	// Relation returns the existing relation with the given id.
	Relation(int) (Relation, error)

	// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
	// with the supplied name (which must be unique across all applications, local and remote).
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)

	// RemoteApplication returns a remote application by name.
	RemoteApplication(string) (RemoteApplication, error)

	// Application returns a local application by name.
	Application(string) (Application, error)

	// WatchRemoteApplications returns a StringsWatcher that notifies of changes to
	// the lifecycles of the remote applications in the model.
	WatchRemoteApplications() state.StringsWatcher

	// WatchRemoteApplicationRelations returns a StringsWatcher that notifies of
	// changes to the lifecycles of relations involving the specified remote
	// application.
	WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error)

	// WatchRemoteRelations returns a StringsWatcher that notifies of changes to
	// the lifecycles of remote relations in the model.
	WatchRemoteRelations() state.StringsWatcher

	// ExportLocalEntity adds an entity to the remote entities collection,
	// returning an opaque token that uniquely identifies the entity within
	// the model.
	ExportLocalEntity(names.Tag) (string, error)

	// GetRemoteEntity returns the tag of the entity associated with the given
	// token and model.
	GetRemoteEntity(names.ModelTag, string) (names.Tag, error)

	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(sourceModel names.ModelTag, entity names.Tag, token string) error

	// GetToken returns the token associated with the entity with the given tag
	// and model.
	GetToken(names.ModelTag, names.Tag) (string, error)

	// ListOffers returns the application offers matching any one of the filter terms.
	ListOffers(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error)
}

// Relation provides access a relation in global state.
type Relation interface {
	// Destroy ensures that the relation will be removed at some point; if
	// no units are currently in scope, it will be removed immediately.
	Destroy() error

	// Id returns the integer internal relation key.
	Id() int

	// Tag returns the relation's tag.
	Tag() names.Tag

	// Life returns the relation's current life state.
	Life() state.Life

	// Endpoints returns the endpoints that constitute the relation.
	Endpoints() []state.Endpoint

	// RemoteUnit returns a RelationUnit for the remote application unit
	// with the supplied ID.
	RemoteUnit(unitId string) (RelationUnit, error)

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchUnits returns a watcher that notifies of changes to the units of the
	// specified application in the relation.
	WatchUnits(applicationName string) (state.RelationUnitsWatcher, error)
}

// RelationUnit provides access to the settings of a single unit in a relation,
// and methods for modifying the unit's involvement in the relation.
type RelationUnit interface {
	// EnterScope ensures that the unit has entered its scope in the
	// relation. When the unit has already entered its scope, EnterScope
	// will report success but make no changes to state.
	EnterScope(settings map[string]interface{}) error

	// InScope returns whether the relation unit has entered scope and
	// not left it.
	InScope() (bool, error)

	// LeaveScope signals that the unit has left its scope in the relation.
	// After the unit has left its relation scope, it is no longer a member
	// of the relation; if the relation is dying when its last member unit
	// leaves, it is removed immediately. It is not an error to leave a
	// scope that the unit is not, or never was, a member of.
	LeaveScope() error

	// ReplaceSettings replaces the relation unit's settings within the
	// relation.
	ReplaceSettings(map[string]interface{}) error

	// Settings returns the relation unit's settings within the relation.
	Settings() (map[string]interface{}, error)
}

// RemoteApplication represents the state of an application hosted in an external
// (remote) model.
type RemoteApplication interface {
	// Name returns the name of the remote application.
	Name() string

	// OfferName returns the name the offering side has given to the remote application..
	OfferName() string

	// Tag returns the remote applications's tag.
	Tag() names.Tag

	// SourceModel returns the tag of the model hosting the remote application.
	SourceModel() names.ModelTag

	// Registered returns the application is created
	// from a registration operation by a consuming model.
	Registered() bool

	// URL returns the remote application URL, at which it is offered.
	URL() (string, bool)

	// Life returns the lifecycle state of the application.
	Life() state.Life

	// Status returns the status of the remote application.
	Status() (status.StatusInfo, error)
}

// Application represents the state of a application hosted in the local model.
type Application interface {
	// Name is the name of the application.
	Name() string

	// Life returns the lifecycle state of the application.
	Life() state.Life

	// Endpoints returns the application's currently available relation endpoints.
	Endpoints() ([]state.Endpoint, error)
}

// Subnet provides access to a subnet in global state.
type Subnet interface {
	// CIDR returns the CIDR of the subnet.
	CIDR() string
}

type statePoolShim struct {
	*state.StatePool
}

func (pool statePoolShim) Get(modelUUID string) (RemoteRelationsState, func(), error) {
	st, closer, err := pool.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return stateShim{st}, closer, nil
}

type stateShim struct {
	*state.State
}

func (st stateShim) AllSubnets() (subnets []Subnet, err error) {
	all, err := st.State.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, s := range all {
		subnets = append(subnets, subnetShim{s})
	}
	return subnets, nil
}

type subnetShim struct {
	*state.Subnet
}

func (a subnetShim) CIDR() string {
	return a.Subnet.CIDR()
}

func (st stateShim) ListOffers(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error) {
	oa := state.NewOfferedApplications(st.State)
	return oa.ListOffers(filter...)
}

func (st stateShim) ExportLocalEntity(entity names.Tag) (string, error) {
	r := st.State.RemoteEntities()
	return r.ExportLocalEntity(entity)
}

func (st stateShim) GetRemoteEntity(model names.ModelTag, token string) (names.Tag, error) {
	r := st.State.RemoteEntities()
	return r.GetRemoteEntity(model, token)
}

func (st stateShim) ImportRemoteEntity(model names.ModelTag, entity names.Tag, token string) error {
	r := st.State.RemoteEntities()
	return r.ImportRemoteEntity(model, entity, token)
}

func (st stateShim) GetToken(model names.ModelTag, entity names.Tag) (string, error) {
	r := st.State.RemoteEntities()
	return r.GetToken(model, entity)
}

func (st stateShim) KeyRelation(key string) (Relation, error) {
	r, err := st.State.KeyRelation(key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r, st.State}, nil
}

func (st stateShim) Relation(id int) (Relation, error) {
	r, err := st.State.Relation(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r, st.State}, nil
}

func (st stateShim) AddRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := st.State.AddRelation(eps...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r, st.State}, nil
}

func (st stateShim) EndpointsRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := st.State.EndpointsRelation(eps...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{r, st.State}, nil
}

func (st stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	a, err := st.State.RemoteApplication(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &remoteApplicationShim{a}, nil
}

func (st stateShim) AddRemoteApplication(args state.AddRemoteApplicationParams) (RemoteApplication, error) {
	a, err := st.State.AddRemoteApplication(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return remoteApplicationShim{a}, nil
}

func (st stateShim) WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error) {
	a, err := st.State.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.WatchRelations(), nil
}

type relationShim struct {
	*state.Relation
	st *state.State
}

func (r relationShim) RemoteUnit(unitId string) (RelationUnit, error) {
	ru, err := r.Relation.RemoteUnit(unitId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationUnitShim{ru}, nil
}

func (r relationShim) Unit(unitId string) (RelationUnit, error) {
	unit, err := r.st.Unit(unitId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ru, err := r.Relation.Unit(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationUnitShim{ru}, nil
}

type relationUnitShim struct {
	*state.RelationUnit
}

func (r relationUnitShim) ReplaceSettings(s map[string]interface{}) error {
	settings, err := r.RelationUnit.Settings()
	if err != nil {
		return errors.Trace(err)
	}
	settings.Update(s)
	for _, key := range settings.Keys() {
		if _, ok := s[key]; ok {
			continue
		}
		settings.Delete(key)
	}
	_, err = settings.Write()
	return errors.Trace(err)
}

func (r relationUnitShim) Settings() (map[string]interface{}, error) {
	settings, err := r.RelationUnit.Settings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}

type remoteApplicationShim struct {
	*state.RemoteApplication
}

type applicationShim struct {
	*state.Application
}

func (st stateShim) Application(name string) (Application, error) {
	a, err := st.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{a}, nil
}
