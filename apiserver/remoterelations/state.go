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

	// WatchRemoteServices returns a StringsWatcher that notifies of changes to
	// the lifecycles of the remote services in the environment.
	WatchRemoteServices() state.StringsWatcher

	// WatchRemoteServiceRelations returns a StringsWatcher that notifies of
	// changes to the lifecycles of relations involving the specified remote
	// service.
	WatchRemoteServiceRelations(serviceName string) (state.StringsWatcher, error)
}

// Relation provides access a relation in global state.
type Relation interface {
	// Id returns the integer internal relation key.
	Id() int

	// Life returns the relation's current life state.
	Life() state.Life

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchCounterpartEndpointUnits returns a watcher that notifies of
	// changes to the units with the endpoint counterpart to the specified
	// service.
	WatchCounterpartEndpointUnits(serviceName string) (state.RelationUnitsWatcher, error)
}

// RelationUnit provides access to the settings of a single unit in a relation.
type RelationUnit interface {
	Settings() (map[string]interface{}, error)
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

func (st stateShim) WatchRemoteServiceRelations(serviceName string) (state.StringsWatcher, error) {
	s, err := st.RemoteService(serviceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.WatchRelations(), nil
}

type relationShim struct {
	*state.Relation
	st *state.State
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

func (r relationUnitShim) Settings() (map[string]interface{}, error) {
	settings, err := r.RelationUnit.Settings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
