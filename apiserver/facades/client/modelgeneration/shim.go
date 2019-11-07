// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/cache"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/state"
)

type modelShim struct {
	*state.Model
}

// Branch wraps the state model branch method,
// returning the locally defined Generation interface.
func (g *modelShim) Branch(name string) (Generation, error) {
	m, err := g.Model.Branch(name)
	return m, errors.Trace(err)
}

// Branches wraps the state model branches method,
// returning a collection of the Generation interface.
func (g *modelShim) Branches() ([]Generation, error) {
	branches, err := g.Model.Branches()
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]Generation, len(branches))
	for i, b := range branches {
		res[i] = b
	}
	return res, nil
}

// CommittedBranch wraps the state model CommittedBranch method,
// returning the locally defined Generation interface.
func (g *modelShim) CommittedBranch(id int) (Generation, error) {
	m, err := g.Model.CommittedBranch(id)
	return m, errors.Trace(err)
}

// Branches wraps the state model CommittedBranches method,
// returning a collection of the Generation interface.
func (g *modelShim) CommittedBranches() ([]Generation, error) {
	branches, err := g.Model.CommittedBranches()
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]Generation, len(branches))
	for i, b := range branches {
		res[i] = b
	}
	return res, nil
}

type applicationShim struct {
	*state.Application
}

// DefaultCharmConfig returns the default configuration
// for this application's charm.
func (a *applicationShim) DefaultCharmConfig() (charm.Settings, error) {
	ch, _, err := a.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ch.Config().DefaultSettings(), nil
}

type stateShim struct {
	*state.State
}

func (st *stateShim) Model() (Model, error) {
	model, err := st.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &modelShim{Model: model}, nil
}

func (st *stateShim) Application(name string) (Application, error) {
	app, err := st.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &applicationShim{Application: app}, nil
}

type modelCacheShim struct {
	*cache.Model
}
