// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Do not register this facade. Resource functionality has been temporarily deleted
// from state so it won't work. State functionality will be replaced by the resource
// domain. Once that is wired up, this facade can be re-registered.
// TODO: Remove the following line when the facade is re-registered.
var _ = Register

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Resources", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

func newFacadeV3(ctx facade.ModelContext) (*API, error) {
	api, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api, nil
}
