// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("InstanceMutater", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*InstanceMutaterAPIV1)(nil)))
	registry.MustRegister("InstanceMutater", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*InstanceMutaterAPI)(nil)))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}

	watcher := &instanceMutatorWatcher{st: st}
	return NewInstanceMutaterAPI(st, watcher, ctx.Resources(), ctx.Auth())
}

// newFacadeV1 is used for API registration.
func newFacadeV1(ctx facade.Context) (*InstanceMutaterAPIV1, error) {
	v2, err := newFacadeV2(ctx)
	if err != nil {
		return nil, err
	}
	return &InstanceMutaterAPIV1{v2}, nil
}
