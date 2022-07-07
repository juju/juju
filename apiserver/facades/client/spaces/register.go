// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/space"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Spaces", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Space API server-side facade with a
// state.State backing.
func newAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	check := common.NewBlockChecker(st)
	callContext := context.CallContext(st)

	reloadSpacesEnvirons, err := DefaultReloadSpacesEnvirons(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	auth := ctx.Auth()
	reloadSpacesAuth := DefaultReloadSpacesAuthorizer(auth, check, stateShim)
	reloadSpacesAPI := NewReloadSpacesAPI(
		space.NewState(st),
		reloadSpacesEnvirons,
		EnvironSpacesAdapter{},
		callContext,
		reloadSpacesAuth,
	)

	return newAPIWithBacking(apiConfig{
		ReloadSpacesAPI: reloadSpacesAPI,
		Backing:         stateShim,
		Check:           check,
		Context:         callContext,
		Resources:       ctx.Resources(),
		Authorizer:      auth,
		Factory:         newOpFactory(st),
	})
}
