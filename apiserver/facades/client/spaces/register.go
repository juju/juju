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
	"github.com/juju/juju/environs/space"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Spaces", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Space API server-side facade with a
// state.State backing.
func newAPI(ctx facade.ModelContext) (*API, error) {
	st := ctx.State()

	serviceFactory := ctx.ServiceFactory()
	cloudService := serviceFactory.Cloud()
	credentialService := serviceFactory.Credential()
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
		space.NewState(st),
		reloadSpacesEnvirons,
		EnvironSpacesAdaptor{},
		credentialInvalidatorGetter,
		reloadSpacesAuth,
	)

	return newAPIWithBacking(apiConfig{
		ReloadSpacesAPI:             reloadSpacesAPI,
		NetworkService:              ctx.ServiceFactory().Network(),
		Backing:                     stateShim,
		Check:                       check,
		CredentialInvalidatorGetter: credentialInvalidatorGetter,
		Resources:                   ctx.Resources(),
		Authorizer:                  auth,
		Factory:                     newOpFactory(st, serviceFactory.ControllerConfig()),
		logger:                      ctx.Logger().Child("spaces"),
	})
}
