// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"reflect"

	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("DiskManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newDiskManagerAPI(ctx)
	}, reflect.TypeOf((*DiskManagerAPI)(nil)))
}

// newDiskManagerAPI creates a new server-side DiskManager API facade.
func newDiskManagerAPI(ctx facade.Context) (*DiskManagerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	authEntityTag := authorizer.GetAuthTag()
	getAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// A machine agent can always access its own machine.
			return tag == authEntityTag
		}, nil
	}

	st := ctx.State()
	return &DiskManagerAPI{
		st:          getState(st),
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}
