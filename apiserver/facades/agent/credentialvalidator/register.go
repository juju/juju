// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/watcher"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialValidator", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newCredentialValidatorAPI(ctx) // adds WatchModelCredential
	}, reflect.TypeOf((*CredentialValidatorAPI)(nil)))
}

// newCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func newCredentialValidatorAPI(ctx facade.ModelContext) (*CredentialValidatorAPI, error) {

	domainServices := ctx.DomainServices()
	modelCredentialWatcherGetter := func(stdCtx context.Context) (watcher.NotifyWatcher, error) {
		return domainServices.Model().WatchModelCloudCredential(stdCtx, ctx.ModelUUID())
	}

	return internalNewCredentialValidatorAPI(
		domainServices.Cloud(),
		domainServices.Credential(),
		ctx.Auth(),
		domainServices.Model(),
		domainServices.ModelInfo(),
		modelCredentialWatcherGetter,
		ctx.WatcherRegistry(),
		ctx.Logger().Child("credentialvalidator"),
	)
}
