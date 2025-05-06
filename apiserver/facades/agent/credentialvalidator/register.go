// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/watcher"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialValidator", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return makeCredentialValidatorAPIV2(ctx) // adds WatchModelCredential
	}, reflect.TypeOf((*CredentialValidatorAPIV2)(nil)))
	registry.MustRegister("CredentialValidator", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return makeCredentialValidatorAPI(ctx) // drops InvalidateCredential
	}, reflect.TypeOf((*CredentialValidatorAPI)(nil)))
}

func makeCredentialValidatorAPIV2(ctx facade.ModelContext) (*CredentialValidatorAPIV2, error) {
	api, err := makeCredentialValidatorAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CredentialValidatorAPIV2{CredentialValidatorAPI: api}, nil
}

// makeCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func makeCredentialValidatorAPI(ctx facade.ModelContext) (*CredentialValidatorAPI, error) {
	authorizer := ctx.Auth()
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	modelCredentialWatcherGetter := func(stdCtx context.Context) (watcher.NotifyWatcher, error) {
		return domainServices.Model().WatchModelCloudCredential(stdCtx, ctx.ModelUUID())
	}

	return NewCredentialValidatorAPI(
		ctx.ModelUUID(),
		&credentialServiceShim{
			modelUUID: ctx.ModelUUID(),
			service:   domainServices.Credential(),
		},
		modelCredentialWatcherGetter,
		ctx.WatcherRegistry(),
	), nil
}
