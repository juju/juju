// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Client", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*ClientV6)(nil)))
	registry.MustRegister("Client", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}

func newFacadeV6(ctx facade.Context) (*ClientV6, error) {
	client, err := newFacadeV7(ctx)
	if err != nil {
		return nil, err
	}
	return &ClientV6{client}, nil
}

func newFacadeV7(ctx facade.Context) (*Client, error) {
	return NewFacade(ctx)
}
