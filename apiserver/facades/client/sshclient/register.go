// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHClient", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

func newFacade(ctx facade.ModelContext) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServices := ctx.DomainServices()
	facadeBackend := backend{
		State:          st,
		networkService: domainServices.Network(),
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{
			Model:              m,
			CloudService:       domainServices.Cloud(),
			CredentialService:  domainServices.Credential(),
			ModelConfigService: domainServices.Config(),
		},
		controllerTag: m.ControllerTag(),
		modelTag:      m.ModelTag(),
	}
	return internalFacade(
		&facadeBackend,
		ctx.DomainServices().Config(),
		ctx.ControllerUUID(),
		leadershipReader,
		ctx.Auth(),
		func(ctx context.Context, args environs.OpenParams) (Broker, error) {
			return caas.New(ctx, args)
		},
	)
}
