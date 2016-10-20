// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/juju/state"
)

// RemoteRelationState provides the subset of global state required by the
// remote relations facade.
type RemoteRelationsState interface {
	// KeyRelation returns the existing relation with the given key (which can
	// be derived unambiguously from the relation's endpoints).
	KeyRelation(string) (Relation, error)

	// Relation returns the existing relation with the given id.
	Relation(int) (Relation, error)

	// RemoteApplication returns a remote application by name.
	RemoteApplication(string) (RemoteApplication, error)

	// WatchRemoteApplications returns a StringsWatcher that notifies of changes to
	// the lifecycles of the remote applications in the environment.
	WatchRemoteApplications() state.StringsWatcher

	// WatchRemoteApplicationRelations returns a StringsWatcher that notifies of
	// changes to the lifecycles of relations involving the specified remote
	// service.
	WatchRemoteApplicationRelations(serviceName string) (state.StringsWatcher, error)
}

// Relation provides access a relation in global state.
type Relation interface {
	// Destroy ensures that the relation will be removed at some point; if
	// no units are currently in scope, it will be removed immediately.
	Destroy() error

	// Id returns the integer internal relation key.
	Id() int

	// Life returns the relation's current life state.
	Life() state.Life

	// RemoteUnit returns a RelationUnit for the remote application unit
	// with the supplied ID.
	RemoteUnit(unitId string) (RelationUnit, error)

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchCounterpartEndpointUnits returns a watcher that notifies of
	// changes to the units with the endpoint counterpart to the specified
	// service.
	WatchCounterpartEndpointUnits(serviceName string) (state.RelationUnitsWatcher, error)
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

// RemoteApplication represents the state of a service hosted in an external
// (remote) environment.
type RemoteApplication interface {
	// Name returns the name of the remote application.
	Name() string

	// URL returns the remote application URL, at which it is offered.
	URL() string
}

type stateShim struct {
	*state.State
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

func (st stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	s, err := st.State.RemoteApplication(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return remoteApplicationShim{s}, nil
}

func (st stateShim) WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error) {
	s, err := st.State.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.WatchRelations(), nil
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
