// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHClient", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx) // v3 adds Leader() method.
	}, reflect.TypeOf((*Facade)(nil)))
}

func newFacade(ctx facade.Context) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipReader, err := ctx.LeadershipReader(m.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalFacade(
		&backend{st, stateenvirons.EnvironConfigGetter{Model: m}, m.ModelTag()},
		leadershipReader,
		ctx.Auth(),
		context.CallContext(st))
}
