// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"reflect"

	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "DiskManager",
		Versions: facades.FacadeVersion{2},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
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
