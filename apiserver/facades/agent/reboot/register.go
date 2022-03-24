// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Reboot", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*RebootAPI)(nil)))
}

// newFacade creates a new server-side RebootAPI facade.
func newFacade(ctx facade.Context) (*RebootAPI, error) {
	return NewRebootAPI(ctx)
}
