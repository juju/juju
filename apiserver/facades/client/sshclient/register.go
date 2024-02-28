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
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHClient", 4, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

func newFacade(ctx facade.ModelContext) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipReader, err := ctx.LeadershipReader(m.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	facadeBackend := backend{
		State: st,
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{
			Model: m, CloudService: ctx.ServiceFactory().Cloud(), CredentialService: ctx.ServiceFactory().Credential()},
		controllerTag: m.ControllerTag(),
		modelTag:      m.ModelTag(),
	}
	return internalFacade(
		&facadeBackend,
		leadershipReader,
		ctx.Auth(),
		func(ctx stdcontext.Context, args environs.OpenParams) (Broker, error) {
			return caas.New(ctx, args)
		},
	)
}
