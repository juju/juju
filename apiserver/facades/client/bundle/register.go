// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Bundle", 8, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV8(ctx)
	}, reflect.TypeOf((*APIv8)(nil)))
}

// newFacadeV8 provides the signature required for facade registration
// for version 8.
func newFacadeV8(ctx facade.ModelContext) (*APIv8, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv8{api}, nil
}
