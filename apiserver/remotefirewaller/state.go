// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	ModelUUID() string

	WatchSubnets(func(id interface{}) bool) state.StringsWatcher

	GetRemoteEntity(model names.ModelTag, token string) (names.Tag, error)

	KeyRelation(string) (Relation, error)

	Application(string) (Application, error)

	Unit(string) (Unit, error)
}

type stateShim struct {
	*state.State
}

func (st stateShim) GetRemoteEntity(model names.ModelTag, token string) (names.Tag, error) {
	r := st.State.RemoteEntities()
	return r.GetRemoteEntity(model, token)
}

func (st stateShim) KeyRelation(key string) (Relation, error) {
	return st.State.KeyRelation(key)
}

type Relation interface {
	Endpoints() []state.Endpoint
	WatchUnits(applicationName string) (state.RelationUnitsWatcher, error)
}

func (st stateShim) Application(name string) (Application, error) {
	app, err := st.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{app}, nil
}

type Application interface {
	Name() string
}

type applicationShim struct {
	*state.Application
}

type Unit interface {
	Name() string
	PublicAddress() (network.Address, error)
}

func (st stateShim) Unit(name string) (Unit, error) {
	return st.State.Unit(name)
}
