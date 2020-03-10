// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
)

type Backend interface {
	Cloud(string) (cloud.Cloud, error)
}

type Pool interface {
	GetModel(string) (Model, func(), error)
}

type Model interface {
	CloudName() string
	EnvironVersion() int
	SetEnvironVersion(int) error
}

func NewPool(pool *state.StatePool) Pool {
	return &statePool{pool}
}

type statePool struct {
	pool *state.StatePool
}

func (p *statePool) GetModel(uuid string) (Model, func(), error) {
	m, ph, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return m, func() { ph.Release() }, nil
}
