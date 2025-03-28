// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHClient", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*FacadeV4)(nil)))
	registry.MustRegister("SSHClient", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx)
	}, reflect.TypeOf((*FacadeV5)(nil)))
}

func newFacadeV5(ctx facade.Context) (*FacadeV5, error) {
	facade, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV5{facade}, nil
}

func newFacadeV4(ctx facade.Context) (*FacadeV4, error) {
	facade, err := newFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV4{facade}, nil
}

func newFacadeBase(ctx facade.Context) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctrlSt, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	leadershipReader, err := ctx.LeadershipReader(m.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	facadeBackend := backend{
		State:               st,
		controllerState:     ctrlSt,
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{Model: m},
		controllerTag:       m.ControllerTag(),
		modelTag:            m.ModelTag(),
	}
	return internalFacade(
		&facadeBackend,
		leadershipReader,
		ctx.Auth(),
		context.CallContext(st),
		func(ctx stdcontext.Context, args environs.OpenParams) (Broker, error) {
			return caas.New(ctx, args)
		},
	)
}
