// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHClient", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV4(stdCtx, ctx)
	}, reflect.TypeFor[*FacadeV4]())
	registry.MustRegister("SSHClient", 5, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV5(stdCtx, ctx)
	}, reflect.TypeFor[*FacadeV5]())
	registry.MustRegister("SSHClient", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV6(stdCtx, ctx)
	}, reflect.TypeFor[*FacadeV6]())
}

func newFacadeV6(stdCtx context.Context, ctx facade.ModelContext) (*FacadeV6, error) {
	facade, err := newFacadeBase(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV6{Facade: facade}, nil
}

func newFacadeV5(stdCtx context.Context, ctx facade.ModelContext) (*FacadeV5, error) {
	facade, err := newFacadeBase(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV5{Facade: facade}, nil
}

func newFacadeV4(stdCtx context.Context, ctx facade.ModelContext) (*FacadeV4, error) {
	facade, err := newFacadeV5(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV4{FacadeV5: facade}, nil
}

func newFacadeBase(stdCtx context.Context, ctx facade.ModelContext) (*Facade, error) {
	domainServices := ctx.DomainServices()
	modelType, err := domainServices.ModelInfo().GetModelType(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalFacade(
		names.NewControllerTag(ctx.ControllerUUID()),
		names.NewModelTag(ctx.ModelUUID().String()),
		modelType,
		domainServices.Application(),
		domainServices.Machine(),
		domainServices.Network(),
		domainServices.Config(),
		domainServices.ModelProvider(),
		domainServices.SSH(),
		ctx.Auth(),
	)
}
