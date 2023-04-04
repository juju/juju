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
		return NewFacade(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}
