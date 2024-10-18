// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASFirewaller", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	domainServices := ctx.DomainServices()
	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         service.NotImplementedSecretService{},
	})

	modelTag := names.NewModelTag(ctx.ModelUUID().String())

	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewFacade(
		resources,
		ctx.WatcherRegistry(),
		authorizer,
		&stateShim{State: ctx.State()},
		commonCharmsAPI,
		appCharmInfoAPI,
		applicationService,
		domainServices.Port(),
	)
}
