// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("InstancePoller", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*InstancePollerAPI)(nil)))
}

// newFacade wraps NewInstancePollerAPI for facade registration.
func newFacade(ctx facade.Context) (*InstancePollerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewInstancePollerAPI(st, m, ctx.Resources(), ctx.Auth(), clock.WallClock)
}
