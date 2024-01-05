// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/pinger"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Pinger", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade provides the signature required for facade registration.
func newFacade(ctx facade.Context) (*API, error) {
	return NewAPI(getPinger(ctx)), nil
}

func getPinger(ctx facade.Context) Pinger {
	worker, err := ctx.WatcherRegistry().Get("pingTimeout")
	if err != nil {
		return pinger.NewNoopPinger()
	}
	if p, ok := worker.(Pinger); ok {
		return p
	}
	return pinger.NewNoopPinger()
}
