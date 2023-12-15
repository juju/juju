// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("LogForwarding", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*LogForwardingAPI)(nil)))
}

// newFacade creates a new LogForwardingAPI. It is used for API registration.
func newFacade(ctx facade.Context) (*LogForwardingAPI, error) {
	return NewLogForwardingAPI(&stateAdaptor{ctx.State()}, ctx.Auth())
}
