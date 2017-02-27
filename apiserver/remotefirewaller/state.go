// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	ModelUUID() string

	WatchSubnets() state.StringsWatcher

	GetRemoteEntity(model names.ModelTag, token string) (names.Tag, error)

	AllSubnets() (subnets []Subnet, err error)

	KeyRelation(string) (Relation, error)

	Application(string) (Application, error)
}

type stateShim struct {
	*state.State
}

func (st stateShim) GetRemoteEntity(model names.ModelTag, token string) (names.Tag, error) {
	r := st.State.RemoteEntities()
	return r.GetRemoteEntity(model, token)
}

func (st stateShim) AllSubnets() (subnets []Subnet, err error) {
	stateSubnets, err := st.State.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, s := range stateSubnets {
		subnets = append(subnets, s)
	}
	return subnets, nil
}

type Subnet interface {
	CIDR() string
}

func (st stateShim) KeyRelation(key string) (Relation, error) {
	return st.State.KeyRelation(key)
}

type Relation interface {
	Endpoints() []state.Endpoint
}

func (st stateShim) Application(name string) (Application, error) {
	return st.State.Application(name)
}

type Application interface {
	Name() string
}
