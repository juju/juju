// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("InstanceMutater", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*InstanceMutaterAPI)(nil)))
	// Bumped to version 4 to include modelUUID in the response struct.
	registry.MustRegister("InstanceMutater", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*InstanceMutaterAPIV4)(nil)))
}

// newFacadeV3 is used for API registration.
func newFacadeV3(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}

	watcher := &instanceMutatorWatcher{st: st}
	return NewInstanceMutaterAPI(st, watcher, ctx.Resources(), ctx.Auth())
}

// newFacadeV4 is used for API registration.
// It includes modelUUID in CharmProfilingInfo response struct.
func newFacadeV4(ctx facade.Context) (*InstanceMutaterAPIV4, error) {
	api, err := newFacadeV3(ctx)
	if err != nil {
		return nil, err
	}
	return &InstanceMutaterAPIV4{api}, nil
}
