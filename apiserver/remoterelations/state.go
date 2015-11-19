// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/juju/state"
)

type RemoteRelationsState interface {
	KeyRelation(string) (Relation, error)
	WatchRemoteServices() state.StringsWatcher
	WatchRemoteServiceRelations(serviceName string) (state.StringsWatcher, error)
}

type Relation interface {
	Id() int
	Life() state.Life
	Unit(unitId string) (RelationUnit, error)
	WatchCounterpartEndpointUnits(serviceName string) (state.RelationUnitsWatcher, error)
}

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
