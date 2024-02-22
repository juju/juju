// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Spaces", 6, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Space API server-side facade with a
// state.State backing.
func newAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	cloudService := ctx.ServiceFactory().Cloud()
	credentialService := ctx.ServiceFactory().Credential()
	stateShim, err := NewStateShim(st, cloudService, credentialService)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credentialInvalidatorGetter := credentialcommon.CredentialInvalidatorGetter(ctx)
	check := common.NewBlockChecker(st)

	reloadSpacesEnvirons, err := DefaultReloadSpacesEnvirons(st, cloudService, credentialService)
	if err != nil {
		return nil, errors.Trace(err)
	}

	auth := ctx.Auth()
	reloadSpacesAuth := DefaultReloadSpacesAuthorizer(auth, check, stateShim)
	reloadSpacesAPI := NewReloadSpacesAPI(
		reloadSpacesStateShim{st},
		reloadSpacesEnvirons,
		EnvironSpacesAdaptor{},
		credentialInvalidatorGetter,
		reloadSpacesAuth,
		ctx.ServiceFactory().Space(),
	)

	return newAPIWithBacking(apiConfig{
		ReloadSpacesAPI:             reloadSpacesAPI,
		Backing:                     stateShim,
		Check:                       check,
		CredentialInvalidatorGetter: credentialInvalidatorGetter,
		Resources:                   ctx.Resources(),
		Authorizer:                  auth,
		Factory:                     newOpFactory(st),
		logger:                      ctx.Logger().Child("spaces"),
	})
}
