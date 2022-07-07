// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Resources", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

func newFacadeV2(ctx facade.Context) (*API, error) {
	api, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api, nil
}
